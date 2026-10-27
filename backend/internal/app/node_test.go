package app_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/app"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/discovery"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/transport/httpapi"
)

func TestEmptyEventListsEncodeAsArrays(t *testing.T) {
	n := app.New("Empty", "127.0.0.1:9", 0)
	data, err := json.Marshal(app.Event{Type: "bundles", Bundles: n.Bundles()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"bundles":[]`) {
		t.Fatalf("empty bundles must encode as [], got %s", data)
	}
	data, err = json.Marshal(app.Event{Type: "peers", Peers: n.Peers()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"peers":[]`) {
		t.Fatalf("empty peers must encode as [], got %s", data)
	}
}

func TestBundleDeliveredViaCarrierAndReceiptReturns(t *testing.T) {
	// Carrier and destination are real HTTP nodes; the sender talks only to the carrier.
	carrier := app.New("Carrier", "127.0.0.1:1", 0)
	dest := app.New("Dest", "127.0.0.1:2", 0)
	carrierSrv := httptest.NewServer(httpapi.New(carrier))
	defer carrierSrv.Close()
	destSrv := httptest.NewServer(httpapi.New(dest))
	defer destSrv.Close()
	carrier.Addr = strings.TrimPrefix(carrierSrv.URL, "http://")
	dest.Addr = strings.TrimPrefix(destSrv.URL, "http://")

	sender := app.New("Sender", "127.0.0.1:3", 0)
	b, err := sender.QueueBundle(dest.ID, "MY-Johor", "hello via carrier", 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	// Sender meets carrier: carrier picks up the bundle, sender keeps half the copies.
	if err := sender.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: carrier.ID, Name: carrier.Name, Addr: carrier.Addr}}); err != nil {
		t.Fatal(err)
	}
	if len(carrier.Bundles()) != 1 {
		t.Fatalf("carrier should hold 1 bundle, holds %d", len(carrier.Bundles()))
	}
	if len(dest.Messages()) != 0 {
		t.Fatal("destination must not receive before meeting the carrier")
	}

	// Carrier meets destination: message is delivered.
	if err := carrier.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: dest.ID, Name: dest.Name, Addr: dest.Addr}}); err != nil {
		t.Fatal(err)
	}
	msgs := dest.Messages()
	if len(msgs) != 1 || msgs[0].Text != "hello via carrier" {
		t.Fatalf("destination did not receive carried message: %+v", msgs)
	}
	// Destination generated a receipt addressed to the sender.
	if len(dest.Bundles()) != 1 {
		t.Fatalf("destination should hold a receipt bundle, holds %d", len(dest.Bundles()))
	}

	// Carrier meets destination again and then the sender: receipt travels back.
	if err := carrier.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: dest.ID, Name: dest.Name, Addr: dest.Addr}}); err != nil {
		t.Fatal(err)
	}
	if err := sender.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: carrier.ID, Name: carrier.Name, Addr: carrier.Addr}}); err != nil {
		t.Fatal(err)
	}
	// Give the receipt path through the carrier one more hop if it is still carried there.
	if len(carrier.Bundles()) > 0 {
		_ = carrier.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: sender.ID, Name: sender.Name, Addr: sender.Addr}})
		_ = sender.ExchangeWith(ctx, discovery.Peer{Beacon: discovery.Beacon{ID: carrier.ID, Name: carrier.Name, Addr: carrier.Addr}})
	}

	found := false
	for _, m := range sender.Messages() {
		if m.ID == b.ID && m.Status == "delivered" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sender never got delivery receipt; messages=%+v carrierBundles=%d", sender.Messages(), len(carrier.Bundles()))
	}
}

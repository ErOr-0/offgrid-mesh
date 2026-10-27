package app_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/app"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/bundles"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/transport/httpapi"
)

func TestDestIPBundleDeliveredWhenIPBecomesReachable(t *testing.T) {
	dest := app.New("Dest", "127.0.0.1:41", 0)
	destSrv := httptest.NewServer(httpapi.New(dest))
	defer destSrv.Close()
	destAddr := strings.TrimPrefix(destSrv.URL, "http://")

	sender := app.New("Sender", "127.0.0.1:42", 0)
	// Bundle addressed only by IP, as if queued in Indonesia for Malaysia.
	b := &bundles.Bundle{
		ID: "ip-bundle", Kind: bundles.KindMessage, From: sender.ID, FromName: "Sender",
		To: "ip:" + destAddr, DestIP: destAddr, Region: "MY-Johor",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Copies: 8,
		Visited: []string{sender.ID}, Payload: []byte("hello Malaysia by IP"),
	}
	if !sender.CarryBundle(b) {
		t.Fatal("bundle should be carried")
	}
	sender.AttemptDestIPDeliveries(context.Background())
	msgs := dest.Messages()
	if len(msgs) != 1 || msgs[0].Text != "hello Malaysia by IP" {
		t.Fatalf("destination did not receive IP-addressed bundle: %+v", msgs)
	}
	if len(sender.Bundles()) != 0 {
		t.Fatalf("sender should hand its last copy to the destination, still carries %d", len(sender.Bundles()))
	}
}

func TestQueueBundleDestIPResolvesAndSeals(t *testing.T) {
	dest := app.New("Dest", "127.0.0.1:43", 0)
	destSrv := httptest.NewServer(httpapi.New(dest))
	defer destSrv.Close()
	destAddr := strings.TrimPrefix(destSrv.URL, "http://")

	sender := app.New("Sender", "127.0.0.1:44", 0)
	b, err := sender.QueueBundleDestIP(context.Background(), destAddr, "MY-Johor", "sealed to IP owner", 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if b.To != dest.ID || b.DestIP != destAddr {
		t.Fatalf("dest IP should resolve to device ID while keeping final address: %+v", b)
	}
	if !b.Encrypted {
		t.Fatal("bundle to a resolved IP should be sealed to that node's key")
	}
}

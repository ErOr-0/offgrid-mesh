package node

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBundleTravelsViaHubBetweenTwoNodes(t *testing.T) {
	hub := New("Hub", "127.0.0.1:10", 0)
	hubSrv := httptest.NewServer(hub.Handler())
	defer hubSrv.Close()
	hubURL := hubSrv.URL

	sender := New("Sender", "127.0.0.1:11", 0)
	receiver := New("Receiver", "127.0.0.1:12", 0)

	ctx := context.Background()
	if _, err := sender.QueueBundle(receiver.ID, "MY-Johor", "via hub when internet appears", 8, time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(receiver.Messages()) != 0 {
		t.Fatal("receiver must have nothing before any sync")
	}

	// Sender gets internet briefly and syncs with the hub.
	if err := sender.AddHub(ctx, hubURL); err != nil {
		t.Fatal(err)
	}
	if len(hub.Bundles()) != 1 {
		t.Fatalf("hub should carry 1 bundle after sender sync, carries %d", len(hub.Bundles()))
	}
	if len(receiver.Messages()) != 0 {
		t.Fatal("receiver must still have nothing: it has not synced yet")
	}

	// Receiver later gets internet and pulls from the hub.
	if err := receiver.AddHub(ctx, hubURL); err != nil {
		t.Fatal(err)
	}
	msgs := receiver.Messages()
	if len(msgs) != 1 || msgs[0].Text != "via hub when internet appears" {
		t.Fatalf("receiver did not get hub bundle: %+v", msgs)
	}
	if !strings.Contains(sender.Hub, "127.0.0.1") {
		t.Fatalf("sender should report its hub, got %q", sender.Hub)
	}
}

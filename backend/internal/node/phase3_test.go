package node

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/discovery"
)

func peerOf(n *Node, srv *httptest.Server) discovery.Peer {
	return discovery.Peer{Beacon: discovery.Beacon{ID: n.ID, Name: n.Name, Addr: strings.TrimPrefix(srv.URL, "http://"), Key: n.PublicKey()}}
}

func TestEncryptedBundleUnreadableByCarrier(t *testing.T) {
	carrier := New("Carrier", "127.0.0.1:21", 0)
	dest := New("Dest", "127.0.0.1:22", 0)
	carrierSrv := httptest.NewServer(carrier.Handler())
	defer carrierSrv.Close()
	destSrv := httptest.NewServer(dest.Handler())
	defer destSrv.Close()
	carrier.Addr = strings.TrimPrefix(carrierSrv.URL, "http://")
	dest.Addr = strings.TrimPrefix(destSrv.URL, "http://")

	sender := New("Sender", "127.0.0.1:23", 0)
	sender.LearnKey(dest.ID, dest.PublicKey())
	b, err := sender.QueueBundle(dest.ID, "", "secret ferry message", 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Encrypted {
		t.Fatal("bundle should be sealed when the destination key is known")
	}
	if bytes.Contains(b.Payload, []byte("secret ferry message")) {
		t.Fatal("sealed payload must not contain plaintext")
	}

	ctx := context.Background()
	if err := sender.ExchangeWith(ctx, peerOf(carrier, carrierSrv)); err != nil {
		t.Fatal(err)
	}
	if len(carrier.Bundles()) != 1 {
		t.Fatalf("carrier should hold the sealed bundle, holds %d", len(carrier.Bundles()))
	}
	if err := carrier.ExchangeWith(ctx, peerOf(dest, destSrv)); err != nil {
		t.Fatal(err)
	}
	msgs := dest.Messages()
	if len(msgs) != 1 || msgs[0].Text != "secret ferry message" || !msgs[0].Encrypted {
		t.Fatalf("destination did not decrypt carried bundle: %+v", msgs)
	}
}

func TestAllowlistRejectsUnpairedSender(t *testing.T) {
	dest := New("Dest", "127.0.0.1:24", 0)
	dest.SetAllowlist([]string{"someone-else"})
	destSrv := httptest.NewServer(dest.Handler())
	defer destSrv.Close()
	dest.Addr = strings.TrimPrefix(destSrv.URL, "http://")

	sender := New("Sender", "127.0.0.1:25", 0)
	carrier := New("Carrier", "127.0.0.1:26", 0)
	carrierSrv := httptest.NewServer(carrier.Handler())
	defer carrierSrv.Close()
	carrier.Addr = strings.TrimPrefix(carrierSrv.URL, "http://")

	if _, err := sender.QueueBundle(dest.ID, "", "unpaired hello", 8, time.Hour); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = sender.ExchangeWith(ctx, peerOf(carrier, carrierSrv))
	_ = carrier.ExchangeWith(ctx, peerOf(dest, destSrv))
	if len(dest.Messages()) != 0 {
		t.Fatalf("unpaired sender must be rejected, dest got %+v", dest.Messages())
	}

	dest.Allow(sender.ID)
	_ = carrier.ExchangeWith(ctx, peerOf(dest, destSrv))
	// A fresh bundle from the now-paired sender is accepted.
	if _, err := sender.QueueBundle(dest.ID, "", "paired hello", 8, time.Hour); err != nil {
		t.Fatal(err)
	}
	_ = sender.ExchangeWith(ctx, peerOf(carrier, carrierSrv))
	_ = carrier.ExchangeWith(ctx, peerOf(dest, destSrv))
	found := false
	for _, m := range dest.Messages() {
		if m.Text == "paired hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("paired sender should be accepted, dest got %+v", dest.Messages())
	}
}

func TestFileTransferAndTunnel(t *testing.T) {
	dest := New("Dest", "127.0.0.1:27", 0)
	destSrv := httptest.NewServer(dest.Handler())
	defer destSrv.Close()
	dest.Addr = strings.TrimPrefix(destSrv.URL, "http://")

	// Dest exposes a tiny local web service.
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<h1>hosted on Dest</h1>")
	}))
	defer site.Close()
	dest.Expose("web", strings.TrimPrefix(site.URL, "http://"))

	sender := New("Sender", "127.0.0.1:28", 0)
	senderSrv := httptest.NewServer(sender.Handler())
	defer senderSrv.Close()
	sender.Addr = strings.TrimPrefix(senderSrv.URL, "http://")
	sender.disc.Upsert(discovery.Beacon{ID: dest.ID, Name: dest.Name, Addr: dest.Addr, Key: dest.PublicKey()})

	ctx := context.Background()
	if _, err := sender.SendFile(ctx, dest.ID, "note.txt", []byte("file carried peer to peer")); err != nil {
		t.Fatal(err)
	}
	got := dest.Files()
	if len(got) != 1 || got[0].Name != "note.txt" {
		t.Fatalf("dest did not receive file: %+v", got)
	}
	resp, err := http.Get(destSrv.URL + "/api/files/" + got[0].ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "file carried peer to peer" {
		t.Fatalf("download mismatch: %q", data)
	}

	// Sender reaches Dest's exposed web service through the tunnel gateway.
	resp2, err := http.Get(senderSrv.URL + "/tunnel/" + dest.ID + "/web/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK || !strings.Contains(string(body), "hosted on Dest") {
		t.Fatalf("tunnel gateway failed: status=%d body=%q", resp2.StatusCode, body)
	}
}

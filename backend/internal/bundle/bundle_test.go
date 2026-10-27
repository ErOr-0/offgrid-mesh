package bundle

import (
	"testing"
	"time"
)

func TestForwardAndExpiry(t *testing.T) {
	now := time.Now()
	b := &Bundle{ID: "1", From: "a", To: "b", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte("hi")}
	if b.Expired(now) {
		t.Fatal("bundle should not be expired")
	}
	next := b.Forward("carrier-1")
	if next.HopCount != 1 || !next.Seen("carrier-1") {
		t.Fatalf("forward did not record carrier: %+v", next)
	}
	if b.HopCount != 0 {
		t.Fatal("Forward must not mutate the original bundle")
	}
	if !b.Expired(now.Add(2 * time.Hour)) {
		t.Fatal("bundle should be expired after TTL")
	}
}

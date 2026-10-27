package bundles

import (
	"testing"
	"time"
)

func TestSprayForSplitsCopies(t *testing.T) {
	now := time.Now()
	b := &Bundle{ID: "1", From: "a", To: "dest", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Copies: 8, Visited: []string{"a"}, Payload: []byte("hi")}

	given, keep, ok := b.SprayFor("carrier-1")
	if !ok {
		t.Fatal("expected spray to a new carrier to be allowed")
	}
	if given.Copies != 4 || keep != 4 {
		t.Fatalf("binary spray should split 8 into 4+4, got given=%d keep=%d", given.Copies, keep)
	}
	if !b.OfferableTo("carrier-2", now) {
		t.Fatal("bundle with copies left should be offerable")
	}

	single := &Bundle{ID: "2", From: "a", To: "dest", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Copies: 1, Visited: []string{"a"}}
	if _, _, ok := single.SprayFor("carrier-1"); ok {
		t.Fatal("last copy must not be sprayed to a non-destination carrier")
	}
	if _, _, ok := single.SprayFor("dest"); !ok {
		t.Fatal("last copy must still be deliverable to the destination")
	}
	if single.OfferableTo("dest", now.Add(2*time.Hour)) {
		t.Fatal("expired bundle must not be offerable")
	}
}

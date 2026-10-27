package bundles

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorePutExpireAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundles.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	live := &Bundle{ID: "live", From: "a", To: "b", ExpiresAt: now.Add(time.Hour), Copies: 4, Payload: []byte("x")}
	dead := &Bundle{ID: "dead", From: "a", To: "b", ExpiresAt: now.Add(-time.Hour), Copies: 4}
	if !s.Put(live) {
		t.Fatal("live bundle should be stored")
	}
	if s.Put(live) {
		t.Fatal("duplicate bundle should be rejected")
	}
	if s.Put(dead) {
		t.Fatal("expired bundle should be rejected")
	}
	if got := s.Expire(now); got != 0 {
		t.Fatalf("nothing stored is expired yet, removed %d", got)
	}
	live.ExpiresAt = now.Add(-time.Minute)
	if err := s.PutForce(live); err != nil {
		t.Fatal(err)
	}
	if got := s.Expire(now); got != 1 || len(s.List()) != 0 {
		t.Fatalf("expiry should remove 1 bundle, removed=%d left=%d", got, len(s.List()))
	}

	if !s.Put(&Bundle{ID: "keep", From: "a", To: "b", ExpiresAt: now.Add(time.Hour), Copies: 2}) {
		t.Fatal("keep bundle should be stored")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.List()) != 1 || reopened.List()[0].ID != "keep" {
		t.Fatalf("persisted bundle not reloaded: %+v", reopened.List())
	}
}

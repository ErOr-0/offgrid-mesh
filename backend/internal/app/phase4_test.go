package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/app"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/bundles"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/tunnel"
)

func TestEncounterRoutingScoresAndGates(t *testing.T) {
	n := app.New("Router", "127.0.0.1:31", 0)
	if got := n.RoutingScore("carrier", "dest", "", ""); got != 0 {
		t.Fatalf("unknown carrier should score 0, got %v", got)
	}
	n.RecordEncounter("carrier")
	first := n.RoutingScore("carrier", "dest", "", "")
	n.RecordEncounter("carrier")
	second := n.RoutingScore("carrier", "dest", "", "")
	if !(second > first && first > 0) {
		t.Fatalf("repeat encounters should raise score: %v then %v", first, second)
	}
	if got := n.RoutingScore("dest", "dest", "", ""); got != 1 {
		t.Fatalf("destination itself should score 1, got %v", got)
	}
	if got := n.RoutingScore("carrier", "dest", "MY-Johor", "MY-Johor"); got <= second {
		t.Fatalf("region match should boost score: base %v boosted %v", second, got)
	}

	// A carried bundle (hop > 0) is not sprayed to a total stranger.
	b := &bundles.Bundle{
		ID: "b1", Kind: bundles.KindMessage, From: "a", To: "dest", Region: "MY-Johor",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), HopCount: 2, Copies: 8,
		Visited: []string{"a", "x"}, Payload: []byte("hi"),
	}
	if !n.CarryBundle(b) {
		t.Fatal("bundle should be stored")
	}
	if offers := n.PrepareOffers("stranger"); len(offers) != 0 {
		t.Fatalf("carried bundle must not spray to unknown carrier, offered %d", len(offers))
	}
	if offers := n.PrepareOffers("carrier"); len(offers) != 1 {
		t.Fatalf("known carrier should receive a spray copy, got %d", len(offers))
	}
	if offers := n.PrepareOffers("dest"); len(offers) != 1 {
		t.Fatalf("destination should always receive the bundle, got %d", len(offers))
	}
}

func TestPairCodeRoundTrip(t *testing.T) {
	a := app.New("A", "127.0.0.1:32", 0)
	code, err := a.PairCode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(code, a.ID) || !strings.Contains(code, a.Fingerprint()) {
		t.Fatalf("pair code should carry id and fingerprint: %s", code)
	}
	b := app.New("B", "127.0.0.1:33", 0)
	if err := b.Pair(code); err != nil {
		t.Fatal(err)
	}
	if !b.HasKey(a.ID) {
		t.Fatal("pairing should learn A's public key")
	}
	if !b.Allowed(a.ID) && len(b.Allowlist()) == 0 {
		t.Fatal("pairing should add A to the allowlist")
	}
	found := false
	for _, id := range b.Allowlist() {
		if id == a.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("pairing should add A to the allowlist")
	}
}

func TestHostDirServesStaticSite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>Offline site</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := app.New("Host", "127.0.0.1:34", 0)
	n.HostDir("site", dir)
	res, err := n.CallExposed(t.Context(), tunnel.Call{Service: "site", Method: "GET", Path: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || !strings.Contains(string(res.Body), "Offline site") {
		t.Fatalf("hosted site not served: status=%d body=%q", res.Status, res.Body)
	}
	if _, err := n.CallExposed(t.Context(), tunnel.Call{Service: "site", Method: "GET", Path: "/../secret.txt"}); err == nil {
		t.Fatal("path traversal outside the hosted directory must fail")
	}
}

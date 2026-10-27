package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/discovery"
)

// BaseURL exports baseURL for other transports.
func BaseURL(addr string) string { return baseURL(addr) }

func baseURL(addr string) string {
	addr = strings.TrimSuffix(addr, "/")
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

// AddHub registers an always-on hub node and exchanges bundles with it now.
// The hub is then a normal manual peer, so the exchange loop keeps syncing
// whenever this node can reach it. All sync is outbound from this node:
// the hub never needs to dial back, which keeps NAT and firewalls happy.
func (n *Node) AddHub(ctx context.Context, hubURL string) error {
	base := baseURL(hubURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/me", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reach hub: %w", err)
	}
	defer resp.Body.Close()
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return err
	}
	n.Hub = base
	n.LearnKey(me.ID, me.Key)
	peer := discovery.Peer{Beacon: discovery.Beacon{ID: me.ID, Name: me.Name, Addr: base, Key: me.Key}}
	n.disc.Upsert(peer.Beacon)
	n.fetchDirectory(ctx, base)
	// Announce ourselves so the hub knows this device exists.
	announce, _ := json.Marshal(discovery.Beacon{ID: n.ID, Name: n.Name, Addr: n.Addr, Key: n.PublicKey()})
	if req2, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/announce", bytes.NewReader(announce)); err == nil {
		req2.Header.Set("Content-Type", "application/json")
		if resp2, err := http.DefaultClient.Do(req2); err == nil {
			resp2.Body.Close()
		}
	}
	exCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return n.ExchangeWith(exCtx, peer)
}

// AddManualPeer fetches a peer's identity from its HTTP API and registers it.
func (n *Node) AddManualPeer(ctx context.Context, addr string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(addr)+"/api/me", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Addr string `json:"addr"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return err
	}
	advertised := me.Addr
	if advertised == "" {
		advertised = addr
	}
	n.LearnKey(me.ID, me.Key)
	n.disc.Upsert(discovery.Beacon{ID: me.ID, Name: me.Name, Addr: advertised, Key: me.Key})
	// Announce ourselves back so the peer knows us without broadcast.
	announce, _ := json.Marshal(discovery.Beacon{ID: n.ID, Name: n.Name, Addr: n.Addr, Key: n.PublicKey()})
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(addr)+"/api/announce", bytes.NewReader(announce))
	if err != nil {
		return nil
	}
	req2.Header.Set("Content-Type", "application/json")
	if resp2, err := http.DefaultClient.Do(req2); err == nil {
		resp2.Body.Close()
	}
	return nil
}

// Send delivers text to a peer over HTTP and records it locally.
func (n *Node) fetchDirectory(ctx context.Context, base string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/directory", nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var entries []discovery.Beacon
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return
	}
	for _, e := range entries {
		n.LearnKey(e.ID, e.Key)
	}
}

// --- Files ---

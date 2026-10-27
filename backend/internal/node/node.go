// Package node is a single OffGrid Mesh node: identity, chat, and UI events.
package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/bundle"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/discovery"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/store"
)

type Message struct {
	ID        string    `json:"id"`
	FromID    string    `json:"from_id"`
	FromName  string    `json:"from_name"`
	ToID      string    `json:"to_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Mine      bool      `json:"mine"`
	Status    string    `json:"status"` // sent | queued | delivered | received
}

type Event struct {
	Type    string           `json:"type"` // message | peers | peer_joined | bundles | delivery
	Message *Message         `json:"message,omitempty"`
	Peers   []discovery.Peer `json:"peers"` // no omitempty: empty must stay [], not be dropped
	Peer    *discovery.Peer  `json:"peer,omitempty"`
	Bundles []*bundle.Bundle `json:"bundles"` // no omitempty: empty must stay [], not be dropped
}

type Node struct {
	ID   string
	Name string
	Addr string // advertised host:port
	Hub  string // base URL of the sync hub, if configured

	disc  *discovery.Service
	store *store.Store

	mu        sync.RWMutex
	messages  []Message
	delivered map[string]bool
	clients   map[*websocket.Conn]struct{}
}

// New creates an in-memory node. Use NewWithData to persist carried bundles.
func New(name, addr string, discPort int) *Node {
	n, _ := NewWithData(name, addr, discPort, "")
	return n
}

func NewWithData(name, addr string, discPort int, dataPath string) (*Node, error) {
	return NewWithID(name, addr, discPort, dataPath, "")
}

// NewWithID is NewWithData with a stable device ID. A destination ID must
// survive restarts, or carried bundles can never find it again.
func NewWithID(name, addr string, discPort int, dataPath, id string) (*Node, error) {
	st, err := store.Open(dataPath)
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = newID()
	}
	n := &Node{ID: id, Name: name, Addr: addr, store: st, delivered: make(map[string]bool), clients: make(map[*websocket.Conn]struct{})}
	n.disc = discovery.New(discovery.Beacon{ID: n.ID, Name: name, Addr: addr}, discPort, func(p discovery.Peer) {
		n.broadcast(Event{Type: "peer_joined", Peer: &p})
		n.broadcastPeers()
	})
	return n, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (n *Node) Run(ctx context.Context) {
	n.disc.Run(ctx)
	go n.exchangeLoop(ctx)
	go n.expireLoop(ctx)
}

func (n *Node) expireLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if n.store.Expire(now) > 0 {
				n.broadcastBundles()
			}
		}
	}
}

func (n *Node) exchangeLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, p := range n.Peers() {
				exCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				_ = n.ExchangeWith(exCtx, p)
				cancel()
			}
		}
	}
}

func (n *Node) Peers() []discovery.Peer { return n.disc.Peers() }

func (n *Node) peerByID(id string) (discovery.Peer, bool) {
	for _, p := range n.disc.Peers() {
		if p.ID == id {
			return p, true
		}
	}
	return discovery.Peer{}, false
}

// baseURL turns a peer address into a base URL. Peers may be plain
// host:port (LAN) or a full http(s):// URL (a hub over the internet).
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return err
	}
	n.Hub = base
	peer := discovery.Peer{Beacon: discovery.Beacon{ID: me.ID, Name: me.Name, Addr: base}}
	n.disc.Upsert(peer.Beacon)
	// Announce ourselves so the hub knows this device exists.
	announce, _ := json.Marshal(discovery.Beacon{ID: n.ID, Name: n.Name, Addr: n.Addr})
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return err
	}
	advertised := me.Addr
	if advertised == "" {
		advertised = addr
	}
	n.disc.Upsert(discovery.Beacon{ID: me.ID, Name: me.Name, Addr: advertised})
	// Announce ourselves back so the peer knows us without broadcast.
	announce, _ := json.Marshal(discovery.Beacon{ID: n.ID, Name: n.Name, Addr: n.Addr})
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
// The payload travels inside a bundle.Bundle, as it will in mule mode.
func (n *Node) Send(ctx context.Context, toID, text string) (*Message, error) {
	peer, ok := n.peerByID(toID)
	if !ok {
		return nil, fmt.Errorf("peer %s not found", toID)
	}
	msg := Message{ID: newID(), FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: time.Now(), Mine: true, Status: "sent"}
	b := bundle.Bundle{
		ID: msg.ID, Kind: bundle.KindMessage, From: n.ID, FromName: n.Name, To: toID, CreatedAt: msg.CreatedAt,
		ExpiresAt: msg.CreatedAt.Add(48 * time.Hour), Copies: 1, Visited: []string{n.ID}, Payload: []byte(text),
	}
	payload, _ := json.Marshal(struct {
		Message Message       `json:"message"`
		Bundle  bundle.Bundle `json:"bundle"`
	}{msg, b})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(peer.Addr)+"/api/receive", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deliver to %s: %w", peer.Addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer returned %s", resp.Status)
	}
	n.storeMessage(msg)
	return &msg, nil
}

func (n *Node) receive(msg Message) {
	msg.Mine = false
	if msg.Status == "" {
		msg.Status = "received"
	}
	n.storeMessage(msg)
}

func (n *Node) storeMessage(msg Message) {
	n.mu.Lock()
	n.messages = append(n.messages, msg)
	n.mu.Unlock()
	n.broadcast(Event{Type: "message", Message: &msg})
}

func (n *Node) Messages() []Message {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return append([]Message{}, n.messages...)
}

func (n *Node) broadcastPeers() {
	n.broadcast(Event{Type: "peers", Peers: n.Peers()})
}

func (n *Node) broadcast(ev Event) {
	if ev.Peers == nil {
		ev.Peers = []discovery.Peer{}
	}
	if ev.Bundles == nil {
		ev.Bundles = []*bundle.Bundle{}
	}
	n.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(n.clients))
	for c := range n.clients {
		clients = append(clients, c)
	}
	n.mu.RUnlock()
	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = wsjson.Write(ctx, c, ev)
		cancel()
	}
}

// Bundles returns bundles this node is carrying.
func (n *Node) Bundles() []*bundle.Bundle { return n.store.List() }

func (n *Node) broadcastBundles() {
	n.broadcast(Event{Type: "bundles", Bundles: n.Bundles()})
}

// QueueBundle creates a message bundle for a destination that may be offline.
// Carriers will exchange it whenever they meet a suitable peer.
func (n *Node) QueueBundle(toID, region, text string, copies int, ttl time.Duration) (*bundle.Bundle, error) {
	if toID == "" || text == "" {
		return nil, fmt.Errorf("to and text are required")
	}
	if copies < 1 {
		copies = 8
	}
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	now := time.Now()
	b := &bundle.Bundle{
		ID: newID(), Kind: bundle.KindMessage, From: n.ID, FromName: n.Name, To: toID, Region: region,
		CreatedAt: now, ExpiresAt: now.Add(ttl), Copies: copies, Visited: []string{n.ID}, Payload: []byte(text),
	}
	if !n.store.Put(b) {
		return nil, fmt.Errorf("bundle rejected")
	}
	msg := Message{ID: b.ID, FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: now, Mine: true, Status: "queued"}
	n.storeMessage(msg)
	n.broadcastBundles()
	return b, nil
}

// ingestBundle accepts a bundle from a carrier: delivers it if addressed
// here, consumes receipts, and otherwise carries it onward.
func (n *Node) ingestBundle(in *bundle.Bundle) {
	if in == nil || in.Expired(time.Now()) || in.Seen(n.ID) {
		return
	}
	if in.To == n.ID {
		switch in.Kind {
		case bundle.KindReceipt:
			n.handleReceipt(in)
		default:
			n.deliver(in)
		}
		return
	}
	if n.store.Put(in.Forward(n.ID)) {
		n.broadcastBundles()
	}
}

func (n *Node) deliver(b *bundle.Bundle) {
	n.mu.Lock()
	if n.delivered[b.ID] {
		n.mu.Unlock()
		return
	}
	n.delivered[b.ID] = true
	n.mu.Unlock()

	msg := Message{
		ID: b.ID, FromID: b.From, FromName: b.FromName, ToID: n.ID,
		Text: string(b.Payload), CreatedAt: b.CreatedAt, Mine: false, Status: "received",
	}
	n.storeMessage(msg)

	now := time.Now()
	receipt := &bundle.Bundle{
		ID: newID(), Kind: bundle.KindReceipt, From: n.ID, FromName: n.Name, To: b.From, Ref: b.ID,
		CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour), Copies: 8, Visited: []string{n.ID}, Payload: []byte(b.ID),
	}
	if b.From != n.ID && n.store.Put(receipt) {
		n.broadcastBundles()
	}
}

func (n *Node) handleReceipt(b *bundle.Bundle) {
	n.mu.Lock()
	changed := false
	for i := range n.messages {
		if n.messages[i].ID == b.Ref && n.messages[i].Status != "delivered" {
			n.messages[i].Status = "delivered"
			msg := n.messages[i]
			changed = true
			defer n.broadcast(Event{Type: "delivery", Message: &msg})
		}
	}
	n.mu.Unlock()
	if changed {
		n.broadcastBundles()
	}
}

// prepareOffers sprays stored bundles for peerID, reducing local copy
// counts. Bundles handed directly to their destination are removed locally;
// a delivery receipt will confirm arrival.
func (n *Node) prepareOffers(peerID string) []*bundle.Bundle {
	var out []*bundle.Bundle
	for _, b := range n.store.OffersFor(peerID, time.Now()) {
		given, keep, ok := b.SprayFor(peerID)
		if !ok {
			continue
		}
		n.store.SetCopies(b.ID, keep)
		out = append(out, given)
	}
	if len(out) > 0 {
		n.broadcastBundles()
	}
	return out
}

type exchangeRequest struct {
	FromID   string           `json:"from_id"`
	FromName string           `json:"from_name"`
	Bundles  []*bundle.Bundle `json:"bundles"`
}

// ExchangeWith swaps offerable bundles with one peer.
func (n *Node) ExchangeWith(ctx context.Context, peer discovery.Peer) error {
	reqBody, _ := json.Marshal(exchangeRequest{FromID: n.ID, FromName: n.Name, Bundles: n.prepareOffers(peer.ID)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(peer.Addr)+"/api/exchange", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer returned %s", resp.Status)
	}
	var out exchangeRequest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	for _, b := range out.Bundles {
		n.ingestBundle(b)
	}
	return nil
}

// Handler returns the node's HTTP API.
func (n *Node) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"id": n.ID, "name": n.Name, "addr": n.Addr, "hub": n.Hub})
	})
	mux.HandleFunc("GET /api/peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Peers())
	})
	mux.HandleFunc("GET /api/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Messages())
	})
	mux.HandleFunc("POST /api/send", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To   string `json:"to"`
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.To == "" || in.Text == "" {
			http.Error(w, "to and text are required", http.StatusBadRequest)
			return
		}
		msg, err := n.Send(r.Context(), in.To, in.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, msg)
	})
	mux.HandleFunc("GET /api/bundles", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Bundles())
	})
	mux.HandleFunc("POST /api/bundles", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To       string `json:"to"`
			Region   string `json:"region"`
			Text     string `json:"text"`
			Copies   int    `json:"copies"`
			TTLHours int    `json:"ttl_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid bundle request", http.StatusBadRequest)
			return
		}
		b, err := n.QueueBundle(in.To, in.Region, in.Text, in.Copies, time.Duration(in.TTLHours)*time.Hour)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, b)
	})
	mux.HandleFunc("POST /api/exchange", func(w http.ResponseWriter, r *http.Request) {
		var in exchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.FromID == "" {
			http.Error(w, "invalid exchange", http.StatusBadRequest)
			return
		}
		for _, b := range in.Bundles {
			n.ingestBundle(b)
		}
		writeJSON(w, exchangeRequest{FromID: n.ID, FromName: n.Name, Bundles: n.prepareOffers(in.FromID)})
	})
	mux.HandleFunc("POST /api/announce", func(w http.ResponseWriter, r *http.Request) {
		var b discovery.Beacon
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.ID == "" {
			http.Error(w, "invalid beacon", http.StatusBadRequest)
			return
		}
		n.disc.Upsert(b)
		n.broadcastPeers()
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/receive", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Message Message       `json:"message"`
			Bundle  bundle.Bundle `json:"bundle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Message.ID == "" {
			http.Error(w, "invalid message", http.StatusBadRequest)
			return
		}
		if in.Bundle.Expired(time.Now()) {
			http.Error(w, "bundle expired", http.StatusGone)
			return
		}
		n.receive(in.Message)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		n.mu.Lock()
		n.clients[c] = struct{}{}
		n.mu.Unlock()
		defer func() {
			n.mu.Lock()
			delete(n.clients, c)
			n.mu.Unlock()
			c.Close(websocket.StatusNormalClosure, "")
		}()
		_ = wsjson.Write(r.Context(), c, Event{Type: "peers", Peers: n.Peers()})
		_ = wsjson.Write(r.Context(), c, Event{Type: "bundles", Bundles: n.Bundles()})
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	})
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

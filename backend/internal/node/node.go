// Package node is a single OffGrid Mesh node: identity, chat, and UI events.
package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/bundle"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/discovery"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/e2e"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/files"
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
	Encrypted bool      `json:"encrypted,omitempty"`
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

	Region string // home region hint, e.g. MY-Johor

	disc    *discovery.Service
	store   *store.Store
	ident   *e2e.Identity
	files   *files.Store
	routing *routingTable

	mu        sync.RWMutex
	messages  []Message
	delivered map[string]bool
	keys      map[string]string // device ID -> X25519 public key hex
	allow     map[string]bool   // if non-empty, only these senders are accepted
	exposes   map[string]string // tunnel service name -> local address
	hostDirs  map[string]string // tunnel service name -> static directory
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
	ident, err := loadIdentity(dataPath)
	if err != nil {
		return nil, err
	}
	n := &Node{
		ID: id, Name: name, Addr: addr, store: st, ident: ident, files: files.New(), routing: newRoutingTable(),
		delivered: make(map[string]bool), keys: make(map[string]string),
		allow: make(map[string]bool), exposes: make(map[string]string), hostDirs: make(map[string]string), clients: make(map[*websocket.Conn]struct{}),
	}
	n.disc = discovery.New(discovery.Beacon{ID: n.ID, Name: name, Addr: addr, Key: ident.PublicHex()}, discPort, func(p discovery.Peer) {
		if p.Key != "" {
			n.LearnKey(p.ID, p.Key)
		}
		n.routing.Encounter(p.ID)
		n.broadcast(Event{Type: "peer_joined", Peer: &p})
		n.broadcastPeers()
	})
	return n, nil
}

// loadIdentity loads the X25519 private key next to the data file, or
// generates one. Without a data file the identity is per-run.
func loadIdentity(dataPath string) (*e2e.Identity, error) {
	if dataPath == "" {
		return e2e.Generate()
	}
	keyPath := dataPath + ".key"
	if raw, err := os.ReadFile(keyPath); err == nil {
		keyBytes, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, err
		}
		return e2e.IdentityFromBytes(keyBytes)
	}
	ident, err := e2e.Generate()
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(keyPath, []byte(hex.EncodeToString(ident.Bytes())), 0o600)
	return ident, nil
}

// SetRegion sets this node's home region, advertised to peers for routing.
func (n *Node) SetRegion(region string) {
	n.Region = region
	n.disc.SetRegion(region)
}

// RoutingEntries returns encounter scores for known carriers.
func (n *Node) RoutingEntries() []Entry { return n.routing.Entries() }

// PairCode returns a JSON pairing code with this node's identity.
// Share it out-of-band (QR, text) so another device can pair and seal to you.
func (n *Node) PairCode() (string, error) {
	data, err := json.Marshal(map[string]string{
		"id": n.ID, "name": n.Name, "key": n.PublicKey(), "fingerprint": n.Fingerprint(),
	})
	return string(data), err
}

// Pair consumes another device's pairing code: learns its key and allowlists it.
func (n *Node) Pair(code string) error {
	var in struct {
		ID          string `json:"id"`
		Key         string `json:"key"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal([]byte(code), &in); err != nil || in.ID == "" || in.Key == "" {
		return fmt.Errorf("invalid pairing code")
	}
	// Verify the fingerprint matches the key so a swapped code is visible.
	if in.Fingerprint != "" {
		sum := sha256.Sum256(mustHex(in.Key))
		if hex.EncodeToString(sum[:])[:8] != in.Fingerprint {
			return fmt.Errorf("pairing code fingerprint does not match its key")
		}
	}
	n.LearnKey(in.ID, in.Key)
	n.Allow(in.ID)
	return nil
}

func mustHex(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

// PublicKey returns this node's X25519 public key, hex encoded.
func (n *Node) PublicKey() string { return n.ident.PublicHex() }

// Fingerprint returns the short pairing code for this node.
func (n *Node) Fingerprint() string { return n.ident.Fingerprint() }

// LearnKey records a device's public key for end-to-end encryption.
func (n *Node) LearnKey(id, pubHex string) {
	if id == "" || pubHex == "" {
		return
	}
	n.mu.Lock()
	n.keys[id] = pubHex
	n.mu.Unlock()
}

func (n *Node) keyFor(id string) (string, bool) {
	n.mu.RLock()
	k, ok := n.keys[id]
	n.mu.RUnlock()
	if ok {
		return k, true
	}
	for _, p := range n.disc.Peers() {
		if p.ID == id && p.Key != "" {
			n.LearnKey(id, p.Key)
			return p.Key, true
		}
	}
	return "", false
}

// seal encrypts text for a device when its key is known.
func (n *Node) seal(toID, text string) ([]byte, bool) {
	pub, ok := n.keyFor(toID)
	if !ok {
		return []byte(text), false
	}
	sealed, err := e2e.Encrypt(pub, []byte(text))
	if err != nil {
		return []byte(text), false
	}
	return sealed, true
}

// SetAllowlist restricts accepted senders. An empty list allows everyone.
func (n *Node) SetAllowlist(ids []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.allow = make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			n.allow[id] = true
		}
	}
}

// Allow adds one device to the allowlist (pairing).
func (n *Node) Allow(id string) { n.SetAllowlist(append(n.Allowlist(), id)) }

// Allowlist returns the current allowlist.
func (n *Node) Allowlist() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make([]string, 0, len(n.allow))
	for id := range n.allow {
		out = append(out, id)
	}
	return out
}

func (n *Node) allowed(fromID string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.allow) == 0 || n.allow[fromID]
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
// The payload travels inside a bundle.Bundle, as it will in mule mode.
func (n *Node) Send(ctx context.Context, toID, text string) (*Message, error) {
	peer, ok := n.peerByID(toID)
	if !ok {
		return nil, fmt.Errorf("peer %s not found", toID)
	}
	sealed, encrypted := n.seal(toID, text)
	msg := Message{ID: newID(), FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: time.Now(), Mine: true, Status: "sent", Encrypted: encrypted}
	b := bundle.Bundle{
		ID: msg.ID, Kind: bundle.KindMessage, From: n.ID, FromName: n.Name, To: toID, CreatedAt: msg.CreatedAt,
		ExpiresAt: msg.CreatedAt.Add(48 * time.Hour), Copies: 1, Visited: []string{n.ID}, Encrypted: encrypted, Payload: sealed,
	}
	wire := msg
	wire.Text = "" // text travels only inside the (sealed) bundle payload
	payload, _ := json.Marshal(struct {
		Message Message       `json:"message"`
		Bundle  bundle.Bundle `json:"bundle"`
	}{wire, b})
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
	sealed, encrypted := n.seal(toID, text)
	b := &bundle.Bundle{
		ID: newID(), Kind: bundle.KindMessage, From: n.ID, FromName: n.Name, To: toID, Region: region,
		CreatedAt: now, ExpiresAt: now.Add(ttl), Copies: copies, Visited: []string{n.ID}, Encrypted: encrypted, Payload: sealed,
	}
	if !n.store.Put(b) {
		return nil, fmt.Errorf("bundle rejected")
	}
	msg := Message{ID: b.ID, FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: now, Mine: true, Status: "queued", Encrypted: encrypted}
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
	if in.From != "" && !n.allowed(in.From) {
		return // sender is not paired with this device
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

	text := string(b.Payload)
	if b.Encrypted {
		plain, err := n.ident.Decrypt(b.Payload)
		if err != nil {
			text = "[encrypted message that cannot be decrypted on this device]"
		} else {
			text = string(plain)
		}
	}
	msg := Message{
		ID: b.ID, FromID: b.From, FromName: b.FromName, ToID: n.ID,
		Text: text, CreatedAt: b.CreatedAt, Mine: false, Status: "received", Encrypted: b.Encrypted,
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
	peerRegion := ""
	if p, ok := n.peerByID(peerID); ok {
		peerRegion = p.Region
	}
	var out []*bundle.Bundle
	for _, b := range n.store.OffersFor(peerID, time.Now()) {
		// Encounter gate: a bundle already carried by others (hop > 0) is
		// only sprayed to carriers with a real encounter score, or whose
		// region matches the destination. Origin bundles spray on meeting.
		if b.To != peerID && b.HopCount > 0 && n.routing.Score(peerID, b.To, peerRegion, b.Region) < 0.1 {
			continue
		}
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
	n.routing.Encounter(peer.ID)
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

// fetchDirectory learns public keys of devices the hub knows. Keys only:
// those devices are not made peers, so we never dial their private addresses.
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

// SendFile transfers a file directly to a known peer.
func (n *Node) SendFile(ctx context.Context, toID, name string, data []byte) (*files.File, error) {
	peer, ok := n.peerByID(toID)
	if !ok {
		return nil, fmt.Errorf("peer %s not found", toID)
	}
	f := files.File{ID: newID(), FromID: n.ID, FromName: n.Name, Name: name, Size: len(data), CreatedAt: time.Now(), Mine: true, Data: data}
	payload, _ := json.Marshal(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(peer.Addr)+"/api/files/receive", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer returned %s", resp.Status)
	}
	n.files.Put(&f)
	return &f, nil
}

func (n *Node) Files() []*files.File { return n.files.List() }

// --- Tunnel (HTTP services through a peer) ---

// Expose makes a local HTTP service reachable by peers under a name.
func (n *Node) Expose(name, localAddr string) {
	n.mu.Lock()
	n.exposes[name] = localAddr
	n.mu.Unlock()
}

func (n *Node) exposed(name string) (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	addr, ok := n.exposes[name]
	return addr, ok
}

type tunnelCall struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Body    []byte `json:"body,omitempty"`
}

type tunnelResult struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

// HostDir serves a static directory as a tunnel service.
func (n *Node) HostDir(name, dir string) {
	n.mu.Lock()
	n.hostDirs[name] = dir
	n.mu.Unlock()
}

func (n *Node) hostDir(name string) (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	dir, ok := n.hostDirs[name]
	return dir, ok
}

var staticTypes = map[string]string{
	".html": "text/html", ".css": "text/css", ".js": "text/javascript",
	".json": "application/json", ".png": "image/png", ".jpg": "image/jpeg",
	".svg": "image/svg+xml", ".txt": "text/plain", ".ico": "image/x-icon",
}

func serveHosted(dir, urlPath string) (*tunnelResult, error) {
	if strings.Contains(urlPath, "..") {
		return nil, fmt.Errorf("path %q escapes the hosted directory", urlPath)
	}
	clean := path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	if clean == "/" || clean == "." {
		clean = "/index.html"
	}
	full := filepath.Join(dir, strings.TrimPrefix(clean, "/"))
	rel, err := filepath.Rel(dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("path %q escapes the hosted directory", urlPath)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return &tunnelResult{Status: http.StatusNotFound, ContentType: "text/plain", Body: []byte("not found")}, nil
	}
	ct := staticTypes[strings.ToLower(filepath.Ext(full))]
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &tunnelResult{Status: http.StatusOK, ContentType: ct, Body: data}, nil
}

// callExposed proxies one HTTP request to a locally exposed service.
func (n *Node) callExposed(ctx context.Context, call tunnelCall) (*tunnelResult, error) {
	if dir, ok := n.hostDir(call.Service); ok {
		return serveHosted(dir, call.Path)
	}
	addr, ok := n.exposed(call.Service)
	if !ok {
		return nil, fmt.Errorf("service %q is not exposed by %s", call.Service, n.Name)
	}
	if !strings.HasPrefix(call.Path, "/") {
		call.Path = "/" + call.Path
	}
	req, err := http.NewRequestWithContext(ctx, call.Method, "http://"+addr+call.Path, bytes.NewReader(call.Body))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return &tunnelResult{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
}

// Handler returns the node's HTTP API.
func (n *Node) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"id": n.ID, "name": n.Name, "addr": n.Addr, "hub": n.Hub,
			"key": n.PublicKey(), "fingerprint": n.Fingerprint(),
		})
	})
	mux.HandleFunc("GET /api/directory", func(w http.ResponseWriter, r *http.Request) {
		// Key directory: devices this node knows, for sealing bundles to
		// destinations that are not currently nearby. Includes this node.
		out := []discovery.Beacon{{ID: n.ID, Name: n.Name, Addr: n.Addr, Key: n.PublicKey()}}
		for _, p := range n.Peers() {
			out = append(out, p.Beacon)
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/routing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.RoutingEntries())
	})
	mux.HandleFunc("GET /api/pair-code", func(w http.ResponseWriter, r *http.Request) {
		code, err := n.PairCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"code": code})
	})
	mux.HandleFunc("GET /api/pair-qr", func(w http.ResponseWriter, r *http.Request) {
		code, err := n.PairCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		png, err := qrcode.Encode(code, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("POST /api/pair", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" {
			http.Error(w, "code is required", http.StatusBadRequest)
			return
		}
		if err := n.Pair(in.Code); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, n.Allowlist())
	})
	mux.HandleFunc("GET /api/allowlist", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Allowlist())
	})
	mux.HandleFunc("POST /api/allow", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		n.Allow(in.ID)
		writeJSON(w, n.Allowlist())
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
		n.LearnKey(b.ID, b.Key)
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
		if !n.allowed(in.Bundle.From) {
			http.Error(w, "sender is not paired with this device", http.StatusForbidden)
			return
		}
		// Text is reconstructed from the bundle payload (decrypted if sealed).
		msg := in.Message
		msg.Text = ""
		n.ingestBundle(&in.Bundle)
		if in.Bundle.To == n.ID {
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}
		n.receive(msg)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/files", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Files())
	})
	mux.HandleFunc("POST /api/files/send", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To   string `json:"to"`
			Name string `json:"name"`
			Data []byte `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(&in); err != nil || in.To == "" || in.Name == "" {
			http.Error(w, "to, name and data are required", http.StatusBadRequest)
			return
		}
		f, err := n.SendFile(r.Context(), in.To, in.Name, in.Data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		meta := *f
		meta.Data = nil
		writeJSON(w, meta)
	})
	mux.HandleFunc("POST /api/files/receive", func(w http.ResponseWriter, r *http.Request) {
		var f files.File
		if err := json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(&f); err != nil || f.ID == "" {
			http.Error(w, "invalid file", http.StatusBadRequest)
			return
		}
		if !n.allowed(f.FromID) {
			http.Error(w, "sender is not paired with this device", http.StatusForbidden)
			return
		}
		f.Mine = false
		f.Size = len(f.Data)
		n.files.Put(&f)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/files/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		f, ok := n.files.Get(r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename="+f.Name)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(f.Data)
	})
	mux.HandleFunc("POST /api/tunnel-call", func(w http.ResponseWriter, r *http.Request) {
		var call tunnelCall
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, "invalid tunnel call", http.StatusBadRequest)
			return
		}
		res, err := n.callExposed(r.Context(), call)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, res)
	})
	// Gateway: /tunnel/{peerID}/{service}/... proxies an HTTP request to a
	// service that peer exposed. This is how a host shares a local site/API.
	mux.HandleFunc("/tunnel/{peerID}/{service}/", func(w http.ResponseWriter, r *http.Request) {
		peer, ok := n.peerByID(r.PathValue("peerID"))
		if !ok {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}
		prefix := "/tunnel/" + r.PathValue("peerID") + "/" + r.PathValue("service")
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		call := tunnelCall{Service: r.PathValue("service"), Method: r.Method, Path: strings.TrimPrefix(r.URL.Path, prefix), Body: body}
		if r.URL.RawQuery != "" {
			call.Path += "?" + r.URL.RawQuery
		}
		payload, _ := json.Marshal(call)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, baseURL(peer.Addr)+"/api/tunnel-call", bytes.NewReader(payload))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg, _ := io.ReadAll(resp.Body)
			http.Error(w, string(msg), http.StatusBadGateway)
			return
		}
		var res tunnelResult
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if res.ContentType != "" {
			w.Header().Set("Content-Type", res.ContentType)
		}
		w.WriteHeader(res.Status)
		_, _ = w.Write(res.Body)
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

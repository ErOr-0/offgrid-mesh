// package app is a single OffGrid Mesh node: identity, chat, and UI events.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/bundles"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/discovery"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/files"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/identity"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/pairing"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/routing"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/tunnel"
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
	Type    string            `json:"type"` // message | peers | peer_joined | bundles | delivery
	Message *Message          `json:"message,omitempty"`
	Peers   []discovery.Peer  `json:"peers"` // no omitempty: empty must stay [], not be dropped
	Peer    *discovery.Peer   `json:"peer,omitempty"`
	Bundles []*bundles.Bundle `json:"bundles"` // no omitempty: empty must stay [], not be dropped
}

type Node struct {
	ID   string
	Name string
	Addr string // advertised host:port
	Hub  string // base URL of the sync hub, if configured

	Region string // home region hint, e.g. MY-Johor

	disc    *discovery.Service
	bundles *bundles.Store
	ident   *identity.Identity
	files   *files.Store
	routing *routing.Table
	tunnel  *tunnel.Service
	allow   *pairing.Allowlist

	mu        sync.RWMutex
	messages  []Message
	delivered map[string]bool
	keys      map[string]string // device ID -> X25519 public key hex
	sink      func(Event)       // transport event sink (WebSocket hub), set by httpapi
}

// SetEventSink registers where node events are published (e.g. WebSocket clients).
func (n *Node) SetEventSink(sink func(Event)) { n.sink = sink }

// UpsertPeer registers a peer beacon from any transport.
func (n *Node) UpsertPeer(b discovery.Beacon) { n.disc.Upsert(b) }

// BroadcastPeers publishes the current peer list to event sinks.
func (n *Node) BroadcastPeers() { n.broadcastPeers() }

// Allowed reports whether a sender passes this node's allowlist.
func (n *Node) Allowed(fromID string) bool { return n.allowed(fromID) }

// PeerByID returns a known peer by device ID.
func (n *Node) PeerByID(id string) (discovery.Peer, bool) { return n.peerByID(id) }

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
	st, err := bundles.Open(dataPath)
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
		ID: id, Name: name, Addr: addr, bundles: st, ident: ident, files: files.New(), routing: routing.New(),
		tunnel: tunnel.New(name), allow: pairing.NewAllowlist(),
		delivered: make(map[string]bool), keys: make(map[string]string),
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
func loadIdentity(dataPath string) (*identity.Identity, error) {
	if dataPath == "" {
		return identity.Generate()
	}
	keyPath := dataPath + ".key"
	if raw, err := os.ReadFile(keyPath); err == nil {
		keyBytes, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, err
		}
		return identity.IdentityFromBytes(keyBytes)
	}
	ident, err := identity.Generate()
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
func (n *Node) RoutingEntries() []routing.Entry { return n.routing.Entries() }

// RoutingScore estimates how good a peer is as next carrier toward a destination.
func (n *Node) RoutingScore(peerID, destID, peerRegion, bundleRegion string) float64 {
	return n.routing.Score(peerID, destID, peerRegion, bundleRegion)
}

// PairCode returns a JSON pairing code with this node's identity.
// Share it out-of-band (QR, text) so another device can pair and seal to you.

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
			if n.bundles.Expire(now) > 0 {
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
			n.AttemptDestIPDeliveries(ctx)
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
// The payload travels inside a bundles.Bundle, as it will in mule mode.
// those devices are not made peers, so we never dial their private addresses.

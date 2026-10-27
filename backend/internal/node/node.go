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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/bundle"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/discovery"
)

type Message struct {
	ID        string    `json:"id"`
	FromID    string    `json:"from_id"`
	FromName  string    `json:"from_name"`
	ToID      string    `json:"to_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	Mine      bool      `json:"mine"`
}

type Event struct {
	Type    string           `json:"type"` // message | peers | peer_joined
	Message *Message         `json:"message,omitempty"`
	Peers   []discovery.Peer `json:"peers,omitempty"`
	Peer    *discovery.Peer  `json:"peer,omitempty"`
}

type Node struct {
	ID   string
	Name string
	Addr string // advertised host:port

	disc *discovery.Service

	mu       sync.RWMutex
	messages []Message
	clients  map[*websocket.Conn]struct{}
}

func New(name, addr string, discPort int) *Node {
	n := &Node{ID: newID(), Name: name, Addr: addr, clients: make(map[*websocket.Conn]struct{})}
	n.disc = discovery.New(discovery.Beacon{ID: n.ID, Name: name, Addr: addr}, discPort, func(p discovery.Peer) {
		n.broadcast(Event{Type: "peer_joined", Peer: &p})
		n.broadcastPeers()
	})
	return n
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (n *Node) Run(ctx context.Context) { n.disc.Run(ctx) }

func (n *Node) Peers() []discovery.Peer { return n.disc.Peers() }

func (n *Node) peerByID(id string) (discovery.Peer, bool) {
	for _, p := range n.disc.Peers() {
		if p.ID == id {
			return p, true
		}
	}
	return discovery.Peer{}, false
}

// AddManualPeer fetches a peer's identity from its HTTP API and registers it.
func (n *Node) AddManualPeer(ctx context.Context, addr string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/me", nil)
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
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/announce", bytes.NewReader(announce))
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
	msg := Message{ID: newID(), FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: time.Now(), Mine: true}
	b := bundle.Bundle{
		ID: msg.ID, From: n.ID, To: toID, CreatedAt: msg.CreatedAt,
		ExpiresAt: msg.CreatedAt.Add(48 * time.Hour), Payload: []byte(text),
	}
	payload, _ := json.Marshal(struct {
		Message Message       `json:"message"`
		Bundle  bundle.Bundle `json:"bundle"`
	}{msg, b})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+peer.Addr+"/api/receive", bytes.NewReader(payload))
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
	n.store(msg)
	return &msg, nil
}

func (n *Node) receive(msg Message) {
	msg.Mine = false
	n.store(msg)
}

func (n *Node) store(msg Message) {
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

// Handler returns the node's HTTP API.
func (n *Node) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"id": n.ID, "name": n.Name, "addr": n.Addr})
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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/bundles"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/discovery"
)

func (n *Node) Send(ctx context.Context, toID, text string) (*Message, error) {
	peer, ok := n.peerByID(toID)
	if !ok {
		return nil, fmt.Errorf("peer %s not found", toID)
	}
	sealed, encrypted := n.seal(toID, text)
	msg := Message{ID: newID(), FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: time.Now(), Mine: true, Status: "sent", Encrypted: encrypted}
	b := bundles.Bundle{
		ID: msg.ID, Kind: bundles.KindMessage, From: n.ID, FromName: n.Name, To: toID, CreatedAt: msg.CreatedAt,
		ExpiresAt: msg.CreatedAt.Add(48 * time.Hour), Copies: 1, Visited: []string{n.ID}, Encrypted: encrypted, Payload: sealed,
	}
	wire := msg
	wire.Text = "" // text travels only inside the (sealed) bundle payload
	payload, _ := json.Marshal(struct {
		Message Message        `json:"message"`
		Bundle  bundles.Bundle `json:"bundle"`
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

// ReceiveDirect records a directly received message (transport fallback).
func (n *Node) ReceiveDirect(msg Message) { n.receive(msg) }

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
		ev.Bundles = []*bundles.Bundle{}
	}
	if n.sink != nil {
		n.sink(ev)
	}
}

// Bundles returns bundles this node is carrying.
func (n *Node) Bundles() []*bundles.Bundle { return n.bundles.List() }

// CarryBundle stores a bundle this node agrees to carry.
func (n *Node) CarryBundle(b *bundles.Bundle) bool { return n.bundles.Put(b) }

// RecordEncounter notes a meeting with a device for routing scores.
func (n *Node) RecordEncounter(id string) { n.routing.Encounter(id) }

// IngestBundle accepts a bundle from a transport (peer exchange, direct receive).
func (n *Node) IngestBundle(b *bundles.Bundle) { n.ingestBundle(b) }

// PrepareOffers sprays stored bundles for a peer (used by the exchange transport).
func (n *Node) PrepareOffers(peerID string) []*bundles.Bundle { return n.prepareOffers(peerID) }

func (n *Node) broadcastBundles() {
	n.broadcast(Event{Type: "bundles", Bundles: n.Bundles()})
}

// QueueBundle creates a message bundle for a destination that may be offline.
// Carriers will exchange it whenever they meet a suitable peer.
func (n *Node) QueueBundle(toID, region, text string, copies int, ttl time.Duration) (*bundles.Bundle, error) {
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
	b := &bundles.Bundle{
		ID: newID(), Kind: bundles.KindMessage, From: n.ID, FromName: n.Name, To: toID, Region: region,
		CreatedAt: now, ExpiresAt: now.Add(ttl), Copies: copies, Visited: []string{n.ID}, Encrypted: encrypted, Payload: sealed,
	}
	if !n.bundles.Put(b) {
		return nil, fmt.Errorf("bundle rejected")
	}
	msg := Message{ID: b.ID, FromID: n.ID, FromName: n.Name, ToID: toID, Text: text, CreatedAt: now, Mine: true, Status: "queued", Encrypted: encrypted}
	n.storeMessage(msg)
	n.broadcastBundles()
	return b, nil
}

// resolveDestIP asks a node at destIP who it is. It succeeds only when that
// IP:port is reachable right now, which is exactly the final-hop condition.
func (n *Node) resolveDestIP(ctx context.Context, destIP string) (id, key string, ok bool) {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, baseURL(destIP)+"/api/me", nil)
	if err != nil {
		return "", "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	var me struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil || me.ID == "" {
		return "", "", false
	}
	n.LearnKey(me.ID, me.Key)
	n.UpsertPeer(discovery.Beacon{ID: me.ID, Name: me.Name, Addr: destIP, Key: me.Key})
	return me.ID, me.Key, true
}

// QueueBundleDestIP queues a bundle addressed to an IP:port instead of a
// device ID. If that node is reachable now, the bundle is sealed to its real
// device ID. Otherwise it travels addressed to the IP, and whichever carrier
// first reaches that IP resolves and delivers it.
func (n *Node) QueueBundleDestIP(ctx context.Context, destIP, region, text string, copies int, ttl time.Duration) (*bundles.Bundle, error) {
	toID := "ip:" + destIP
	if id, _, ok := n.resolveDestIP(ctx, destIP); ok {
		toID = id
	}
	b, err := n.QueueBundle(toID, region, text, copies, ttl)
	if err != nil {
		return nil, err
	}
	// Record the final delivery address (and re-seal if resolution raced).
	stored, _ := n.bundles.Get(b.ID)
	if stored != nil {
		stored.DestIP = destIP
		_ = n.bundles.PutForce(stored)
		b.DestIP = destIP
	}
	return b, nil
}

// AttemptDestIPDeliveries probes the destination IP of carried bundles and
// delivers any whose IP has become reachable. Called on every exchange tick.
func (n *Node) AttemptDestIPDeliveries(ctx context.Context) {
	for _, b := range n.Bundles() {
		if b.DestIP == "" {
			continue
		}
		id, _, ok := n.resolveDestIP(ctx, b.DestIP)
		if !ok {
			continue
		}
		if b.To != id {
			n.bundles.SetDestination(b.ID, id)
		}
		if peer, ok := n.peerByID(id); ok {
			_ = n.ExchangeWith(ctx, peer)
		}
	}
}

// ingestBundle accepts a bundle from a carrier: delivers it if addressed
// here, consumes receipts, and otherwise carries it onward.
func (n *Node) ingestBundle(in *bundles.Bundle) {
	if in == nil || in.Expired(time.Now()) || in.Seen(n.ID) {
		return
	}
	if in.From != "" && !n.allowed(in.From) {
		return // sender is not paired with this device
	}
	if in.To == n.ID {
		switch in.Kind {
		case bundles.KindReceipt:
			n.handleReceipt(in)
		default:
			n.deliver(in)
		}
		return
	}
	if n.bundles.Put(in.Forward(n.ID)) {
		n.broadcastBundles()
	}
}

func (n *Node) deliver(b *bundles.Bundle) {
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
	receipt := &bundles.Bundle{
		ID: newID(), Kind: bundles.KindReceipt, From: n.ID, FromName: n.Name, To: b.From, Ref: b.ID,
		CreatedAt: now, ExpiresAt: now.Add(48 * time.Hour), Copies: 8, Visited: []string{n.ID}, Payload: []byte(b.ID),
	}
	if b.From != n.ID && n.bundles.Put(receipt) {
		n.broadcastBundles()
	}
}

func (n *Node) handleReceipt(b *bundles.Bundle) {
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
func (n *Node) prepareOffers(peerID string) []*bundles.Bundle {
	peerRegion := ""
	if p, ok := n.peerByID(peerID); ok {
		peerRegion = p.Region
	}
	var out []*bundles.Bundle
	for _, b := range n.bundles.OffersFor(peerID, time.Now()) {
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
		n.bundles.SetCopies(b.ID, keep)
		out = append(out, given)
	}
	if len(out) > 0 {
		n.broadcastBundles()
	}
	return out
}

// ExchangeRequest is the peer bundle-exchange wire format.
type ExchangeRequest = exchangeRequest

type exchangeRequest struct {
	FromID   string            `json:"from_id"`
	FromName string            `json:"from_name"`
	Bundles  []*bundles.Bundle `json:"bundles"`
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

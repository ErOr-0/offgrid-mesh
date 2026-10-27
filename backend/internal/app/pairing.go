package app

import (
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/identity"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/pairing"
)

// PairCode returns this node's pairing code (see pairing module).
func (n *Node) PairCode() (string, error) {
	return pairing.BuildCode(n.ID, n.Name, n.PublicKey(), n.Fingerprint())
}

// Pair consumes another device's pairing code: learns its key and allowlists it.
func (n *Node) Pair(code string) error {
	in, err := pairing.ParseCode(code)
	if err != nil {
		return err
	}
	n.LearnKey(in.ID, in.Key)
	n.Allow(in.ID)
	return nil
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

// HasKey reports whether this node knows a device's public key.
func (n *Node) HasKey(id string) bool {
	_, ok := n.keyFor(id)
	return ok
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
	sealed, err := identity.Encrypt(pub, []byte(text))
	if err != nil {
		return []byte(text), false
	}
	return sealed, true
}

// SetAllowlist restricts accepted senders. An empty list allows everyone.
func (n *Node) SetAllowlist(ids []string) { n.allow.Set(ids) }

// Allow adds one device to the allowlist (pairing).
func (n *Node) Allow(id string) { n.allow.Add(id) }

// Allowlist returns the current allowlist.
func (n *Node) Allowlist() []string { return n.allow.List() }

func (n *Node) allowed(fromID string) bool { return n.allow.Allowed(fromID) }

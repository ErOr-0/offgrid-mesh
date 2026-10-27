// Package pairing holds device allowlists and pairing codes.
package pairing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// Allowlist restricts which senders a node accepts. Empty allows everyone.
type Allowlist struct {
	mu  sync.RWMutex
	set map[string]bool
}

func NewAllowlist() *Allowlist { return &Allowlist{set: make(map[string]bool)} }

func (a *Allowlist) Set(ids []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.set = make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			a.set[id] = true
		}
	}
}

func (a *Allowlist) Add(id string) {
	a.mu.Lock()
	a.set[id] = true
	a.mu.Unlock()
}

func (a *Allowlist) List() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.set))
	for id := range a.set {
		out = append(out, id)
	}
	return out
}

func (a *Allowlist) Allowed(fromID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.set) == 0 || a.set[fromID]
}

// Code is the JSON pairing payload shared out-of-band (QR or text).
type Code struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

// BuildCode creates a pairing code for a device.
func BuildCode(id, name, pubHex, fingerprint string) (string, error) {
	data, err := json.Marshal(Code{ID: id, Name: name, Key: pubHex, Fingerprint: fingerprint})
	return string(data), err
}

// ParseCode validates a pairing code, including that the fingerprint
// matches the key so a swapped code is visible.
func ParseCode(code string) (*Code, error) {
	var in Code
	if err := json.Unmarshal([]byte(code), &in); err != nil || in.ID == "" || in.Key == "" {
		return nil, fmt.Errorf("invalid pairing code")
	}
	if in.Fingerprint != "" {
		keyBytes, err := hex.DecodeString(in.Key)
		if err != nil {
			return nil, fmt.Errorf("invalid pairing code key")
		}
		sum := sha256.Sum256(keyBytes)
		if hex.EncodeToString(sum[:])[:8] != in.Fingerprint {
			return nil, fmt.Errorf("pairing code fingerprint does not match its key")
		}
	}
	return &in, nil
}

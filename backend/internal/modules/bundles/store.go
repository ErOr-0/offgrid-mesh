// Package bundles holds bundles a node is carrying, optionally on disk.
package bundles

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// Store is a thread-safe bundle queue with TTL expiry and JSON persistence.
type Store struct {
	mu      sync.RWMutex
	path    string
	bundles map[string]*Bundle
}

// Open loads a store from path. An empty path keeps bundles in memory only.
func Open(path string) (*Store, error) {
	s := &Store{path: path, bundles: make(map[string]*Bundle)}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Bundle
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, b := range list {
		s.bundles[b.ID] = b
	}
	return s, nil
}

// Put stores a bundle. It rejects duplicates and expired bundles.
func (s *Store) Put(b *Bundle) bool {
	if b == nil || b.ID == "" || b.Expired(time.Now()) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bundles[b.ID]; exists {
		return false
	}
	s.bundles[b.ID] = clone(b)
	s.saveLocked()
	return true
}

// PutForce stores or replaces a bundle regardless of expiry.
func (s *Store) PutForce(b *Bundle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bundles[b.ID] = clone(b)
	s.saveLocked()
	return nil
}

// Get returns a copy of a stored bundle.
func (s *Store) Get(id string) (*Bundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bundles[id]
	if !ok {
		return nil, false
	}
	return clone(b), true
}

// Delete removes a bundle.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bundles[id]; ok {
		delete(s.bundles, id)
		s.saveLocked()
	}
}

// SetDestination resolves a bundle addressed by IP to a real device ID
// once that IP turns out to belong to a node.
func (s *Store) SetDestination(id, destID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.bundles[id]; ok {
		b.To = destID
		s.saveLocked()
	}
}

// SetCopies updates the Spray-and-Wait copy count of a stored bundle.
// A count of zero or less removes the bundle.
func (s *Store) SetCopies(id string, copies int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bundles[id]
	if !ok {
		return
	}
	if copies <= 0 {
		delete(s.bundles, id)
	} else {
		b.Copies = copies
	}
	s.saveLocked()
}

// List returns copies of all stored bundles, oldest first.
func (s *Store) List() []*Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Bundle, 0, len(s.bundles))
	for _, b := range s.bundles {
		out = append(out, clone(b))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// OffersFor returns stored bundles this node may offer to peerID.
func (s *Store) OffersFor(peerID string, now time.Time) []*Bundle {
	var out []*Bundle
	for _, b := range s.List() {
		if b.OfferableTo(peerID, now) {
			out = append(out, b)
		}
	}
	return out
}

// Expire deletes bundles past their TTL and returns how many were removed.
func (s *Store) Expire(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, b := range s.bundles {
		if b.Expired(now) {
			delete(s.bundles, id)
			removed++
		}
	}
	if removed > 0 {
		s.saveLocked()
	}
	return removed
}

func clone(b *Bundle) *Bundle {
	c := *b
	c.Visited = append([]string{}, b.Visited...)
	c.Payload = append([]byte{}, b.Payload...)
	return &c
}

func (s *Store) saveLocked() {
	if s.path == "" {
		return
	}
	list := make([]*Bundle, 0, len(s.bundles))
	for _, b := range s.bundles {
		list = append(list, b)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, s.path)
	}
}

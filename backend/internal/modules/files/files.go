// Package files stores files received from peers, in memory.
package files

import (
	"sort"
	"sync"
	"time"
)

// File is a received or sent file. List() strips Data, so listings never carry file bytes.
type File struct {
	ID        string    `json:"id"`
	FromID    string    `json:"from_id"`
	FromName  string    `json:"from_name"`
	Name      string    `json:"name"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Mine      bool      `json:"mine"`
	Data      []byte    `json:"data,omitempty"`
}

// Store is a thread-safe in-memory file store.
type Store struct {
	mu    sync.RWMutex
	files map[string]*File
}

func New() *Store { return &Store{files: make(map[string]*File)} }

func (s *Store) Put(f *File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *f
	cp.Data = append([]byte{}, f.Data...)
	s.files[f.ID] = &cp
}

func (s *Store) Get(id string) (*File, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.files[id]
	if !ok {
		return nil, false
	}
	cp := *f
	cp.Data = append([]byte{}, f.Data...)
	return &cp, true
}

// List returns file metadata, newest first.
func (s *Store) List() []*File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*File, 0, len(s.files))
	for _, f := range s.files {
		cp := *f
		cp.Data = nil
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

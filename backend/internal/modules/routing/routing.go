package routing

import (
	"sort"
	"sync"
	"time"
)

// Table tracks encounter-based delivery predictability, in the
// spirit of PRoPHET: devices met often, recently, or heading to the
// bundle's region are better carriers.
type Table struct {
	mu         sync.RWMutex
	pred       map[string]float64
	encounters map[string]int
	lastSeen   map[string]time.Time
}

// New creates an empty routing table.
func New() *Table {
	return &Table{
		pred:       make(map[string]float64),
		encounters: make(map[string]int),
		lastSeen:   make(map[string]time.Time),
	}
}

// Encounter records a meeting and raises that device's predictability.
func (r *Table) Encounter(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pred[id]
	r.pred[id] = p + (1-p)*0.75
	r.encounters[id]++
	r.lastSeen[id] = time.Now()
}

// Score estimates how good peerID is as the next carrier toward destID.
// The destination scores 1. A region match boosts the score.
func (r *Table) Score(peerID, destID, peerRegion, bundleRegion string) float64 {
	if peerID == destID {
		return 1
	}
	r.mu.RLock()
	score := r.pred[peerID]
	r.mu.RUnlock()
	if bundleRegion != "" && peerRegion == bundleRegion {
		score += 0.25
	}
	if score > 1 {
		score = 1
	}
	return score
}

// Entry is one row of the routing table, for display.
type Entry struct {
	DeviceID   string    `json:"device_id"`
	Score      float64   `json:"score"`
	Encounters int       `json:"encounters"`
	LastSeen   time.Time `json:"last_seen"`
}

// Entries returns the routing table, best carriers first.
func (r *Table) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.pred))
	for id, score := range r.pred {
		out = append(out, Entry{DeviceID: id, Score: score, Encounters: r.encounters[id], LastSeen: r.lastSeen[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

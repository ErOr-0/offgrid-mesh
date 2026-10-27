// Package discovery finds peer nodes on the local network with UDP broadcast beacons.
package discovery

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

// Beacon is broadcast periodically so nearby nodes can find each other.
type Beacon struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Addr   string `json:"addr"`             // host:port of the node's HTTP API
	Key    string `json:"key,omitempty"`    // X25519 public key, hex, for end-to-end encryption
	Region string `json:"region,omitempty"` // home region hint, e.g. MY-Johor
}

// Peer is a discovered node.
type Peer struct {
	Beacon
	LastSeen time.Time `json:"last_seen"`
	Manual   bool      `json:"manual,omitempty"` // added by address: never expires
}

// Service broadcasts this node's beacon and tracks beacons from others.
type Service struct {
	self   Beacon
	selfMu sync.RWMutex
	port   int
	onPeer func(Peer)

	mu    sync.RWMutex
	peers map[string]Peer
}

// SetRegion updates the region advertised in this node's beacon.
func (s *Service) SetRegion(region string) {
	s.selfMu.Lock()
	s.self.Region = region
	s.selfMu.Unlock()
}

func New(self Beacon, port int, onPeer func(Peer)) *Service {
	return &Service{self: self, port: port, onPeer: onPeer, peers: make(map[string]Peer)}
}

// Peers returns a snapshot of live peers, excluding expired ones.
func (s *Service) Peers() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	return out
}

// Upsert registers or refreshes a peer found by other means, e.g. a manual
// address. Manual peers are not beacon-refreshed, so they never expire.
func (s *Service) Upsert(b Beacon) { s.addBeacon(b, true) }

// addBeacon registers a peer from any discovery source.
func (s *Service) addBeacon(b Beacon, manual bool) {
	if b.ID == "" || b.ID == s.self.ID {
		return
	}
	p := Peer{Beacon: b, LastSeen: time.Now(), Manual: manual}
	s.mu.Lock()
	old, known := s.peers[b.ID]
	if known && old.Manual {
		p.Manual = true
	}
	s.peers[b.ID] = p
	s.mu.Unlock()
	if !known && s.onPeer != nil {
		s.onPeer(p)
	}
}

// Run broadcasts and listens until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	go s.listen(ctx)
	go s.broadcast(ctx)
	go s.expire(ctx)
	go s.runMDNS(ctx)
}

func (s *Service) broadcast(ctx context.Context) {
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: s.port})
	if err != nil {
		log.Printf("discovery: broadcast dial: %v", err)
		return
	}
	defer conn.Close()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		s.selfMu.RLock()
		data, _ := json.Marshal(s.self)
		s.selfMu.RUnlock()
		if _, err := conn.Write(data); err != nil {
			log.Printf("discovery: broadcast write: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) listen(ctx context.Context) {
	pc, err := net.ListenPacket("udp", net.JoinHostPort("", strconv.Itoa(s.port)))
	if err != nil {
		log.Printf("discovery: listen: %v", err)
		return
	}
	defer pc.Close()
	go func() {
		<-ctx.Done()
		pc.Close()
	}()
	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		var b Beacon
		if err := json.Unmarshal(buf[:n], &b); err != nil || b.ID == "" || b.ID == s.self.ID {
			continue
		}
		// Prefer the beacon's advertised address; fall back to the source IP with its port.
		peerAddr := b.Addr
		if peerAddr == "" {
			if udp, ok := addr.(*net.UDPAddr); ok {
				peerAddr = net.JoinHostPort(udp.IP.String(), portOf(b.Addr))
			}
		}
		b.Addr = peerAddr
		s.addBeacon(b, false)
	}
}

func (s *Service) expire(ctx context.Context) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			for id, p := range s.peers {
				if !p.Manual && now.Sub(p.LastSeen) > 10*time.Second {
					delete(s.peers, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}

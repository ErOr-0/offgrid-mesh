package discovery

import (
	"context"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/grandcat/zeroconf"
)

const mdnsService = "_offgrid-mesh._tcp"

// runMDNS advertises this node and browses for others. It complements UDP
// broadcast: broadcast is fast on one subnet, mDNS works where broadcast is
// filtered. Failure is non-fatal.
func (s *Service) runMDNS(ctx context.Context) {
	s.selfMu.RLock()
	self := s.self
	s.selfMu.RUnlock()

	_, portStr, err := net.SplitHostPort(self.Addr)
	if err != nil {
		log.Printf("discovery: mDNS skipped, bad address %q", self.Addr)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Printf("discovery: mDNS skipped, bad port %q", portStr)
		return
	}
	shortID := self.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	instance := self.Name + "-" + shortID
	server, err := zeroconf.Register(instance, mdnsService, "local.", port, []string{
		"id=" + self.ID, "key=" + self.Key, "region=" + self.Region, "addr=" + self.Addr,
	}, nil)
	if err != nil {
		log.Printf("discovery: mDNS register: %v", err)
		return
	}
	defer server.Shutdown()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("discovery: mDNS resolver: %v", err)
		return
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for entry := range entries {
			b := beaconFromEntry(entry)
			if b.ID != "" {
				s.addBeacon(b, false)
			}
		}
	}()
	if err := resolver.Browse(ctx, mdnsService, "local.", entries); err != nil {
		log.Printf("discovery: mDNS browse: %v", err)
	}
}

func beaconFromEntry(entry *zeroconf.ServiceEntry) Beacon {
	fields := map[string]string{}
	for _, txt := range entry.Text {
		if k, v, ok := strings.Cut(txt, "="); ok {
			fields[k] = v
		}
	}
	addr := fields["addr"]
	if addr == "" && len(entry.AddrIPv4) > 0 {
		addr = net.JoinHostPort(entry.AddrIPv4[0].String(), strconv.Itoa(entry.Port))
	}
	name := entry.Instance
	if i := strings.Index(name, "-"); i > 0 {
		name = name[:i]
	}
	return Beacon{ID: fields["id"], Name: name, Addr: addr, Key: fields["key"], Region: fields["region"]}
}

// Command node runs a single OffGrid Mesh node.
//
// Example (two nodes on one machine):
//
//	go run ./cmd/node -name Alice -port 8080 -discovery-port 42069
//	go run ./cmd/node -name Bob   -port 8081 -discovery-port 42070 -peer localhost:8080
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/node"
)

func main() {
	name := flag.String("name", "node", "display name of this node")
	port := flag.Int("port", 8080, "HTTP API port")
	discPort := flag.Int("discovery-port", 42069, "UDP discovery port")
	peerList := flag.String("peer", "", "comma-separated peer addresses (host:port) if broadcast is blocked")
	dataPath := flag.String("data", "", "file to persist carried bundles, e.g. alice-bundles.json")
	deviceID := flag.String("id", "", "stable device ID (default: generated, persisted next to -data file)")
	hubURL := flag.String("hub", "", "sync hub base URL, e.g. https://hub.example.com or http://192.168.1.10:8090 (syncs when reachable)")
	allowList := flag.String("allow", "", "comma-separated device IDs allowed to send to this node (default: everyone)")
	exposeList := flag.String("expose", "", "comma-separated name=host:port HTTP services to expose to peers, e.g. web=127.0.0.1:3000")
	hostList := flag.String("host", "", "comma-separated name=directory static sites to host for peers, e.g. site=./hosted")
	region := flag.String("region", "", "home region hint for routing, e.g. MY-Johor or ID-Batam")
	flag.Parse()

	id := *deviceID
	if id == "" && *dataPath != "" {
		id = loadOrCreateID(*dataPath + ".id")
	}

	addr := fmt.Sprintf("%s:%d", localIP(), *port)
	n, err := node.NewWithID(*name, addr, *discPort, *dataPath, id)
	if err != nil {
		log.Fatalf("load bundle store: %v", err)
	}

	if *allowList != "" {
		n.SetAllowlist(strings.Split(*allowList, ","))
		log.Printf("allowlist active: %v", n.Allowlist())
	}
	for _, item := range strings.Split(*exposeList, ",") {
		if parts := strings.SplitN(strings.TrimSpace(item), "=", 2); len(parts) == 2 && parts[0] != "" {
			n.Expose(parts[0], parts[1])
			log.Printf("exposing service %q at %s to peers", parts[0], parts[1])
		}
	}
	for _, item := range strings.Split(*hostList, ",") {
		if parts := strings.SplitN(strings.TrimSpace(item), "=", 2); len(parts) == 2 && parts[0] != "" {
			n.HostDir(parts[0], parts[1])
			log.Printf("hosting static site %q from %s", parts[0], parts[1])
		}
	}
	if *region != "" {
		n.SetRegion(*region)
	}
	log.Printf("%s pairing fingerprint: %s", *name, n.Fingerprint())

	ctx := context.Background()
	n.Run(ctx)

	if *hubURL != "" {
		go func() {
			// Keep retrying: the hub syncs whenever connectivity appears.
			for {
				if err := n.AddHub(ctx, *hubURL); err == nil {
					log.Printf("synced with hub %s", *hubURL)
					return
				}
				time.Sleep(10 * time.Second)
			}
		}()
	}

	if *peerList != "" {
		go func() {
			for _, addr := range strings.Split(*peerList, ",") {
				addr = strings.TrimSpace(addr)
				if addr == "" {
					continue
				}
				for i := 0; i < 10; i++ {
					if err := n.AddManualPeer(ctx, addr); err == nil {
						log.Printf("added manual peer %s", addr)
						break
					}
					time.Sleep(time.Second)
				}
			}
		}()
	}

	log.Printf("%s (%s) listening on port %d, advertising %s", *name, n.ID, *port, addr)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), n.Handler()))
}

// loadOrCreateID keeps a node's device ID stable across restarts.
func loadOrCreateID(path string) string {
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return strings.TrimSpace(string(data))
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	_ = os.WriteFile(path, []byte(id), 0o600)
	return id
}

func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if udp, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return udp.IP.String()
	}
	return "127.0.0.1"
}

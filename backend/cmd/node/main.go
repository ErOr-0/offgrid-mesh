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

	ctx := context.Background()
	n.Run(ctx)

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

package app

import (
	"context"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/tunnel"
)

// Tunnel type aliases keep the wire format in the tunnel module.
type tunnelCall = tunnel.Call
type tunnelResult = tunnel.Result

// Expose makes a local HTTP service reachable by peers under a name.
func (n *Node) Expose(name, localAddr string) { n.tunnel.Expose(name, localAddr) }

// HostDir serves a static directory as a tunnel service.
func (n *Node) HostDir(name, dir string) { n.tunnel.HostDir(name, dir) }

func (n *Node) callExposed(ctx context.Context, call tunnelCall) (*tunnelResult, error) {
	return n.tunnel.Call(ctx, call)
}

// CallExposed proxies a request to a service this node exposes (tunnel transport).
func (n *Node) CallExposed(ctx context.Context, call tunnelCall) (*tunnelResult, error) {
	return n.tunnel.Call(ctx, call)
}

// Package httpapi is the HTTP/WebSocket transport adapter for a node.
// It contains no domain logic: every handler delegates to the app module.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/app"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/bundles"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/discovery"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/files"
	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/tunnel"
)

type hub struct {
	node    *app.Node
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func (h *hub) broadcast(ev app.Event) {
	if ev.Peers == nil {
		ev.Peers = []discovery.Peer{}
	}
	if ev.Bundles == nil {
		ev.Bundles = []*bundles.Bundle{}
	}
	h.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = wsjson.Write(ctx, c, ev)
		cancel()
	}
}

// New returns the node's HTTP API and wires the WebSocket event hub.
func New(n *app.Node) http.Handler {
	hub := &hub{node: n, clients: make(map[*websocket.Conn]struct{})}
	n.SetEventSink(hub.broadcast)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{
			"id": n.ID, "name": n.Name, "addr": n.Addr, "hub": n.Hub,
			"key": n.PublicKey(), "fingerprint": n.Fingerprint(),
		})
	})
	mux.HandleFunc("GET /api/directory", func(w http.ResponseWriter, r *http.Request) {
		// Key directory: devices this node knows, for sealing bundles to
		// destinations that are not currently nearby. Includes this node.
		out := []discovery.Beacon{{ID: n.ID, Name: n.Name, Addr: n.Addr, Key: n.PublicKey()}}
		for _, p := range n.Peers() {
			out = append(out, p.Beacon)
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/routing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.RoutingEntries())
	})
	mux.HandleFunc("GET /api/pair-code", func(w http.ResponseWriter, r *http.Request) {
		code, err := n.PairCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"code": code})
	})
	mux.HandleFunc("GET /api/pair-qr", func(w http.ResponseWriter, r *http.Request) {
		code, err := n.PairCode()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		png, err := qrcode.Encode(code, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("POST /api/pair", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" {
			http.Error(w, "code is required", http.StatusBadRequest)
			return
		}
		if err := n.Pair(in.Code); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, n.Allowlist())
	})
	mux.HandleFunc("GET /api/allowlist", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Allowlist())
	})
	mux.HandleFunc("POST /api/allow", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		n.Allow(in.ID)
		writeJSON(w, n.Allowlist())
	})
	mux.HandleFunc("GET /api/peers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Peers())
	})
	mux.HandleFunc("GET /api/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Messages())
	})
	mux.HandleFunc("POST /api/send", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To   string `json:"to"`
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.To == "" || in.Text == "" {
			http.Error(w, "to and text are required", http.StatusBadRequest)
			return
		}
		msg, err := n.Send(r.Context(), in.To, in.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, msg)
	})
	mux.HandleFunc("GET /api/bundles", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Bundles())
	})
	mux.HandleFunc("POST /api/bundles", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To       string `json:"to"`
			DestIP   string `json:"dest_ip"`
			Region   string `json:"region"`
			Text     string `json:"text"`
			Copies   int    `json:"copies"`
			TTLHours int    `json:"ttl_hours"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid bundle request", http.StatusBadRequest)
			return
		}
		var b *bundles.Bundle
		var err error
		if in.DestIP != "" {
			b, err = n.QueueBundleDestIP(r.Context(), in.DestIP, in.Region, in.Text, in.Copies, time.Duration(in.TTLHours)*time.Hour)
		} else {
			b, err = n.QueueBundle(in.To, in.Region, in.Text, in.Copies, time.Duration(in.TTLHours)*time.Hour)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, b)
	})
	mux.HandleFunc("POST /api/exchange", func(w http.ResponseWriter, r *http.Request) {
		var in app.ExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.FromID == "" {
			http.Error(w, "invalid exchange", http.StatusBadRequest)
			return
		}
		for _, b := range in.Bundles {
			n.IngestBundle(b)
		}
		writeJSON(w, app.ExchangeRequest{FromID: n.ID, FromName: n.Name, Bundles: n.PrepareOffers(in.FromID)})
	})
	mux.HandleFunc("POST /api/announce", func(w http.ResponseWriter, r *http.Request) {
		var b discovery.Beacon
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.ID == "" {
			http.Error(w, "invalid beacon", http.StatusBadRequest)
			return
		}
		n.LearnKey(b.ID, b.Key)
		n.UpsertPeer(b)
		n.BroadcastPeers()
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/receive", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Message app.Message    `json:"message"`
			Bundle  bundles.Bundle `json:"bundle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Message.ID == "" {
			http.Error(w, "invalid message", http.StatusBadRequest)
			return
		}
		if in.Bundle.Expired(time.Now()) {
			http.Error(w, "bundle expired", http.StatusGone)
			return
		}
		if !n.Allowed(in.Bundle.From) {
			http.Error(w, "sender is not paired with this device", http.StatusForbidden)
			return
		}
		// Text is reconstructed from the bundle payload (decrypted if sealed).
		msg := in.Message
		msg.Text = ""
		n.IngestBundle(&in.Bundle)
		if in.Bundle.To == n.ID {
			writeJSON(w, map[string]string{"status": "ok"})
			return
		}
		n.ReceiveDirect(msg)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/files", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, n.Files())
	})
	mux.HandleFunc("POST /api/files/send", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			To   string `json:"to"`
			Name string `json:"name"`
			Data []byte `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(&in); err != nil || in.To == "" || in.Name == "" {
			http.Error(w, "to, name and data are required", http.StatusBadRequest)
			return
		}
		f, err := n.SendFile(r.Context(), in.To, in.Name, in.Data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		meta := *f
		meta.Data = nil
		writeJSON(w, meta)
	})
	mux.HandleFunc("POST /api/files/receive", func(w http.ResponseWriter, r *http.Request) {
		var f files.File
		if err := json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(&f); err != nil || f.ID == "" {
			http.Error(w, "invalid file", http.StatusBadRequest)
			return
		}
		if !n.Allowed(f.FromID) {
			http.Error(w, "sender is not paired with this device", http.StatusForbidden)
			return
		}
		n.ReceiveFile(&f)
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/files/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		f, ok := n.GetFile(r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename="+f.Name)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(f.Data)
	})
	mux.HandleFunc("POST /api/tunnel-call", func(w http.ResponseWriter, r *http.Request) {
		var call tunnel.Call
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, "invalid tunnel call", http.StatusBadRequest)
			return
		}
		res, err := n.CallExposed(r.Context(), call)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, res)
	})
	// Gateway: /tunnel/{peerID}/{service}/... proxies an HTTP request to a
	// service that peer exposed. This is how a host shares a local site/API.
	mux.HandleFunc("/tunnel/{peerID}/{service}/", func(w http.ResponseWriter, r *http.Request) {
		peer, ok := n.PeerByID(r.PathValue("peerID"))
		if !ok {
			http.Error(w, "peer not found", http.StatusNotFound)
			return
		}
		prefix := "/tunnel/" + r.PathValue("peerID") + "/" + r.PathValue("service")
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		call := tunnel.Call{Service: r.PathValue("service"), Method: r.Method, Path: strings.TrimPrefix(r.URL.Path, prefix), Body: body}
		if r.URL.RawQuery != "" {
			call.Path += "?" + r.URL.RawQuery
		}
		payload, _ := json.Marshal(call)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, app.BaseURL(peer.Addr)+"/api/tunnel-call", bytes.NewReader(payload))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg, _ := io.ReadAll(resp.Body)
			http.Error(w, string(msg), http.StatusBadGateway)
			return
		}
		var res tunnel.Result
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if res.ContentType != "" {
			w.Header().Set("Content-Type", res.ContentType)
		}
		w.WriteHeader(res.Status)
		_, _ = w.Write(res.Body)
	})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		hub.mu.Lock()
		hub.clients[c] = struct{}{}
		hub.mu.Unlock()
		defer func() {
			hub.mu.Lock()
			delete(hub.clients, c)
			hub.mu.Unlock()
			c.Close(websocket.StatusNormalClosure, "")
		}()
		_ = wsjson.Write(r.Context(), c, app.Event{Type: "peers", Peers: n.Peers()})
		_ = wsjson.Write(r.Context(), c, app.Event{Type: "bundles", Bundles: n.Bundles()})
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	})
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Package tunnel exposes local HTTP services and static sites to peers.
package tunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// Call is one proxied HTTP request to an exposed service.
type Call struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Body    []byte `json:"body,omitempty"`
}

// Result is the proxied response.
type Result struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

// Service holds the services one node exposes.
type Service struct {
	mu       sync.RWMutex
	exposes  map[string]string
	hostDirs map[string]string
	owner    string
}

func New(owner string) *Service {
	return &Service{exposes: make(map[string]string), hostDirs: make(map[string]string), owner: owner}
}

// Expose makes a local HTTP service reachable by peers under a name.
func (s *Service) Expose(name, localAddr string) {
	s.mu.Lock()
	s.exposes[name] = localAddr
	s.mu.Unlock()
}

// HostDir serves a static directory as a tunnel service.
func (s *Service) HostDir(name, dir string) {
	s.mu.Lock()
	s.hostDirs[name] = dir
	s.mu.Unlock()
}

var staticTypes = map[string]string{
	".html": "text/html", ".css": "text/css", ".js": "text/javascript",
	".json": "application/json", ".png": "image/png", ".jpg": "image/jpeg",
	".svg": "image/svg+xml", ".txt": "text/plain", ".ico": "image/x-icon",
}

func serveHosted(dir, urlPath string) (*Result, error) {
	if strings.Contains(urlPath, "..") {
		return nil, fmt.Errorf("path %q escapes the hosted directory", urlPath)
	}
	clean := path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	if clean == "/" || clean == "." {
		clean = "/index.html"
	}
	full := filepath.Join(dir, strings.TrimPrefix(clean, "/"))
	rel, err := filepath.Rel(dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("path %q escapes the hosted directory", urlPath)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return &Result{Status: http.StatusNotFound, ContentType: "text/plain", Body: []byte("not found")}, nil
	}
	ct := staticTypes[strings.ToLower(filepath.Ext(full))]
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &Result{Status: http.StatusOK, ContentType: ct, Body: data}, nil
}

// Call proxies one request to an exposed service or hosted directory.
func (s *Service) Call(ctx context.Context, call Call) (*Result, error) {
	s.mu.RLock()
	dir, isDir := s.hostDirs[call.Service]
	addr, isAddr := s.exposes[call.Service]
	s.mu.RUnlock()
	if isDir {
		return serveHosted(dir, call.Path)
	}
	if !isAddr {
		return nil, fmt.Errorf("service %q is not exposed by %s", call.Service, s.owner)
	}
	if !strings.HasPrefix(call.Path, "/") {
		call.Path = "/" + call.Path
	}
	req, err := http.NewRequestWithContext(ctx, call.Method, "http://"+addr+call.Path, bytes.NewReader(call.Body))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return &Result{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
}

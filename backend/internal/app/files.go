package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ErOr-0/offgrid-mesh/backend/internal/modules/files"
)

func (n *Node) SendFile(ctx context.Context, toID, name string, data []byte) (*files.File, error) {
	peer, ok := n.peerByID(toID)
	if !ok {
		return nil, fmt.Errorf("peer %s not found", toID)
	}
	f := files.File{ID: newID(), FromID: n.ID, FromName: n.Name, Name: name, Size: len(data), CreatedAt: time.Now(), Mine: true, Data: data}
	payload, _ := json.Marshal(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(peer.Addr)+"/api/files/receive", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer returned %s", resp.Status)
	}
	n.files.Put(&f)
	return &f, nil
}

func (n *Node) Files() []*files.File { return n.files.List() }

// ReceiveFile stores a file arriving from a peer transport.
func (n *Node) ReceiveFile(f *files.File) {
	f.Mine = false
	f.Size = len(f.Data)
	n.files.Put(f)
}

// GetFile returns a stored file for download.
func (n *Node) GetFile(id string) (*files.File, bool) { return n.files.Get(id) }

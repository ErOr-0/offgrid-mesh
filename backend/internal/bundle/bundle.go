// Package bundle defines the unit of offline, store-carry-forward delivery.
package bundle

import "time"

// Bundle is an encrypted message plus its routing metadata.
// Carriers may read the metadata to forward the bundle.
// They cannot read Payload, which is end-to-end encrypted.
type Bundle struct {
	ID        string    `json:"id"`
	From      string    `json:"from"` // sender device ID (public-key hash)
	To        string    `json:"to"`   // recipient device ID (public-key hash)
	Region    string    `json:"region,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	HopCount  int       `json:"hop_count"`
	Visited   []string  `json:"visited,omitempty"` // carrier hashes, for loop prevention
	Payload   []byte    `json:"payload"`
}

// Expired reports whether the bundle's TTL has passed.
func (b *Bundle) Expired(now time.Time) bool {
	return now.After(b.ExpiresAt)
}

// Seen reports whether carrierID has already carried this bundle.
func (b *Bundle) Seen(carrierID string) bool {
	for _, v := range b.Visited {
		if v == carrierID {
			return true
		}
	}
	return false
}

// Forward returns a copy of the bundle marked as carried by carrierID.
func (b *Bundle) Forward(carrierID string) *Bundle {
	next := *b
	next.HopCount++
	next.Visited = append(append([]string{}, b.Visited...), carrierID)
	next.Payload = append([]byte{}, b.Payload...)
	return &next
}

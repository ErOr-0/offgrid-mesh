// package bundles defines the unit of offline, store-carry-forward delivery.
package bundles

import "time"

// Kind distinguishes a user message from a delivery receipt.
type Kind string

const (
	KindMessage Kind = "message"
	KindReceipt Kind = "receipt"
)

// Bundle is an encrypted message plus its routing metadata.
// Carriers may read the metadata to forward the bundle.
// They cannot read Payload, which is end-to-end encrypted.
type Bundle struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	From      string    `json:"from"` // sender device ID (public-key hash)
	FromName  string    `json:"from_name,omitempty"`
	To        string    `json:"to"`                // recipient device ID (public-key hash)
	DestIP    string    `json:"dest_ip,omitempty"` // final delivery address (host:port) when known by IP instead of device ID
	Region    string    `json:"region,omitempty"`
	Ref       string    `json:"ref,omitempty"` // receipt: ID of the delivered bundle
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	HopCount  int       `json:"hop_count"`
	Copies    int       `json:"copies"`              // Spray-and-Wait copies this holder may still spray
	Visited   []string  `json:"visited,omitempty"`   // carrier hashes, for loop prevention
	Encrypted bool      `json:"encrypted,omitempty"` // Payload is sealed to To's public key
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

// OfferableTo reports whether this holder may offer the bundle to peerID:
// not expired, peer has not carried it, and either the peer is the
// destination or copies remain to spray.
func (b *Bundle) OfferableTo(peerID string, now time.Time) bool {
	if b.Expired(now) || b.Seen(peerID) {
		return false
	}
	return peerID == b.To || b.Copies > 1
}

// SprayFor splits this holder's copies for peerID (binary Spray-and-Wait).
// It returns the copy to hand over and how many copies this holder keeps.
// The destination always receives a copy, even the holder's last one.
func (b *Bundle) SprayFor(peerID string) (given *Bundle, keep int, ok bool) {
	if b.Seen(peerID) {
		return nil, b.Copies, false
	}
	cp := *b
	cp.Payload = append([]byte{}, b.Payload...)
	cp.Visited = append([]string{}, b.Visited...)
	if peerID == b.To {
		cp.Copies = 1
		return &cp, 0, true
	}
	if b.Copies <= 1 {
		return nil, b.Copies, false
	}
	cp.Copies = b.Copies / 2
	return &cp, b.Copies - cp.Copies, true
}

// package identity seals bundle payloads to a recipient's X25519 public key.
// Carriers and hubs only ever see ciphertext.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Identity is a node's long-term X25519 key pair.
type Identity struct {
	priv *ecdh.PrivateKey
}

// Generate creates a fresh identity.
func Generate() (*Identity, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv}, nil
}

// IdentityFromBytes restores an identity from its raw private key bytes.
func IdentityFromBytes(b []byte) (*Identity, error) {
	priv, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv}, nil
}

// Bytes returns the raw private key for persistence.
func (id *Identity) Bytes() []byte { return id.priv.Bytes() }

// PublicHex is the hex-encoded public key shared with peers.
func (id *Identity) PublicHex() string {
	return hex.EncodeToString(id.priv.PublicKey().Bytes())
}

// Fingerprint is a short human-checkable code for pairing verification.
func (id *Identity) Fingerprint() string {
	sum := sha256.Sum256(id.priv.PublicKey().Bytes())
	return hex.EncodeToString(sum[:])[:8]
}

// envelope is the wire format stored in bundle.Payload.
type envelope struct {
	V     int    `json:"v"`
	EPK   string `json:"epk"`   // ephemeral public key, hex
	Nonce string `json:"nonce"` // AES-GCM nonce, hex
	CT    string `json:"ct"`    // ciphertext, hex
}

func deriveKey(shared []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, shared, nil, "offgrid-mesh-bundle-v1", 32)
}

func sealWithEphemeral(recipientPub *ecdh.PublicKey, ephemeral *ecdh.PrivateKey, plain []byte) ([]byte, error) {
	shared, err := ephemeral.ECDH(recipientPub)
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(shared)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	env := envelope{
		V:     1,
		EPK:   hex.EncodeToString(ephemeral.PublicKey().Bytes()),
		Nonce: hex.EncodeToString(nonce),
		CT:    hex.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}
	return json.Marshal(env)
}

// Encrypt seals plain for the recipient identified by public key hex.
func Encrypt(recipientPubHex string, plain []byte) ([]byte, error) {
	pubBytes, err := hex.DecodeString(recipientPubHex)
	if err != nil {
		return nil, fmt.Errorf("recipient key: %w", err)
	}
	pub, err := ecdh.X25519().NewPublicKey(pubBytes)
	if err != nil {
		return nil, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return sealWithEphemeral(pub, eph, plain)
}

// Decrypt opens a sealed payload. It fails for any other identity or tampering.
func (id *Identity) Decrypt(sealed []byte) ([]byte, error) {
	var env envelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		return nil, errors.New("not an encrypted envelope")
	}
	epkBytes, err := hex.DecodeString(env.EPK)
	if err != nil {
		return nil, err
	}
	epk, err := ecdh.X25519().NewPublicKey(epkBytes)
	if err != nil {
		return nil, err
	}
	shared, err := id.priv.ECDH(epk)
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(shared)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := hex.DecodeString(env.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := hex.DecodeString(env.CT)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

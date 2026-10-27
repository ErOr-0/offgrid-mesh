package identity

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTripAndTamper(t *testing.T) {
	alice, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	bob, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello Malaysia, carriers cannot read this")
	sealed, err := Encrypt(bob.PublicHex(), plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("ciphertext must not contain the plaintext")
	}
	out, err := bob.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("round trip mismatch: %q", out)
	}
	if _, err := alice.Decrypt(sealed); err == nil {
		t.Fatal("a third party must not decrypt a bundle sealed for Bob")
	}
	tampered := append([]byte{}, sealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := bob.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

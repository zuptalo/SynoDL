package store

import (
	"bytes"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("kdf-input-for-tests")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	pt := []byte("nas-password-hunter2")
	ct, err := c.Seal(pt)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ct, pt) {
		t.Fatal("ciphertext must not contain the plaintext")
	}
	got, err := c.Open(ct)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("round-trip mismatch: got %q", got)
	}
}

func TestCipherWrongKeyFails(t *testing.T) {
	a, _ := NewCipher("kdf-a")
	b, _ := NewCipher("kdf-b")
	ct, _ := a.Seal([]byte("secret"))
	if _, err := b.Open(ct); err == nil {
		t.Fatal("decrypting with a different SECRETS_KEY must fail (wrong/missing key)")
	}
}

func TestCipherNonceIsRandom(t *testing.T) {
	c, _ := NewCipher("k")
	x, _ := c.Seal([]byte("same"))
	y, _ := c.Seal([]byte("same"))
	if bytes.Equal(x, y) {
		t.Fatal("two seals of the same plaintext must differ (random nonce)")
	}
}

func TestNewCipherRequiresKey(t *testing.T) {
	if _, err := NewCipher(""); err == nil {
		t.Fatal("empty SECRETS_KEY must error")
	}
}

func TestOpenRejectsShortCiphertext(t *testing.T) {
	c, _ := NewCipher("k")
	if _, err := c.Open([]byte("x")); err == nil {
		t.Fatal("too-short ciphertext must error, not panic")
	}
}

// Package store is SynoDL's persistence layer (constitution v2.0.0, Principle
// III): a single SQLite database on the mounted volume, with secret columns
// encrypted at rest. SECRETS_KEY (operator env) never touches the DB or logs.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// Cipher encrypts secret columns (NAS password, VAPID private key) at rest with
// AES-256-GCM, under a key derived from the operator's SECRETS_KEY via
// HKDF-SHA256. Each Seal uses a fresh random nonce, prepended to the ciphertext.
// Losing SECRETS_KEY makes stored secrets unrecoverable — by design.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives the at-rest key from SECRETS_KEY. An empty key is an error:
// stateful SynoDL MUST NOT run without one.
func NewCipher(secretsKey string) (*Cipher, error) {
	if secretsKey == "" {
		return nil, errors.New("SECRETS_KEY is required")
	}
	// HKDF binds the derived key to a fixed info label so the same SECRETS_KEY
	// could later derive other independent keys without collision.
	key, err := hkdf.Key(sha256.New, []byte(secretsKey), nil, "synodl:secretbox:v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Seal returns nonce||ciphertext||tag.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal. A wrong key or tampered/short input returns an error
// (never a panic) — the caller turns this into a "wrong or missing SECRETS_KEY"
// boot failure rather than silently resetting stored secrets.
func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(sealed) < ns {
		return nil, errors.New("ciphertext too short")
	}
	return c.aead.Open(nil, sealed[:ns], sealed[ns:], nil)
}

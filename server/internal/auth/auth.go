// Package auth handles SynoDL's own account security: salted password hashing
// and opaque session tokens. Stdlib-only (crypto/pbkdf2, Go 1.24+); no external
// dependency. Plaintext passwords and raw tokens are never persisted — the store
// keeps only the encoded hash and the token's hash.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// PBKDF2 parameters. Iteration count follows current OWASP guidance for
// PBKDF2-HMAC-SHA256; it is encoded in the hash so it can rise later without
// invalidating existing hashes.
const (
	pbkdf2Iter = 210_000
	saltLen    = 16
	keyLen     = 32
)

// HashPassword returns an encoded salted PBKDF2-SHA256 hash:
// "pbkdf2-sha256$<iter>$<salt-b64>$<key-b64>".
func HashPassword(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, plain, salt, pbkdf2Iter, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk)), nil
}

// VerifyPassword reports whether plain matches an encoded hash, in constant time.
func VerifyPassword(encoded, plain string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, plain, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewSessionToken returns a random opaque token (handed to the client) and its
// storage hash (kept server-side). The raw token is never persisted.
func NewSessionToken() (token, tokenHash string, err error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken returns the storage hash (SHA-256 hex) of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Package push implements Web Push (RFC 8291 message encryption + RFC 8292
// VAPID) using only the Go standard library — no third-party webpush dependency
// (constitution: minimize server deps). The instance VAPID keypair is generated
// once and stored (private key encrypted); the completion watcher and the push
// endpoints use it to notify opted-in devices when a download finishes.
package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"time"
)

var b64 = base64.RawURLEncoding

// GenerateVAPIDKeys returns a fresh P-256 keypair as base64url strings: the
// public key is the 65-byte uncompressed point, the private key the 32-byte
// scalar.
func GenerateVAPIDKeys() (public, private string, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	pub := elliptic.Marshal(elliptic.P256(), priv.X, priv.Y) //nolint:staticcheck // stdlib-only, no crypto/ecdh point export
	d := make([]byte, 32)
	priv.D.FillBytes(d)
	return b64.EncodeToString(pub), b64.EncodeToString(d), nil
}

// privateKeyFrom reconstructs an ECDSA private key from the 32-byte scalar.
func privateKeyFrom(privateB64 string) (*ecdsa.PrivateKey, error) {
	d, err := b64.DecodeString(privateB64)
	if err != nil {
		return nil, err
	}
	priv := new(ecdsa.PrivateKey)
	priv.Curve = elliptic.P256()
	priv.D = new(big.Int).SetBytes(d)
	priv.X, priv.Y = elliptic.P256().ScalarBaseMult(d) //nolint:staticcheck // stdlib-only
	return priv, nil
}

// authorizationHeader builds the RFC 8292 "vapid t=<jwt>, k=<pubkey>" header for
// a push request to endpoint.
func authorizationHeader(endpoint, subject, publicB64, privateB64 string) (string, error) {
	priv, err := privateKeyFrom(privateB64)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", errors.New("push: bad endpoint URL")
	}
	jwt, err := signVAPIDJWT(priv, u.Scheme+"://"+u.Host, subject)
	if err != nil {
		return "", err
	}
	return "vapid t=" + jwt + ", k=" + publicB64, nil
}

// signVAPIDJWT produces an ES256-signed JWT with the given audience + subject.
func signVAPIDJWT(priv *ecdsa.PrivateKey, aud, subject string) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	exp := time.Now().Add(12 * time.Hour).Unix()
	claims := b64.EncodeToString([]byte(fmt.Sprintf(`{"aud":%q,"exp":%d,"sub":%q}`, aud, exp, subject)))
	signingInput := header + "." + claims
	sum := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		return "", err
	}
	// JWS ES256 signature is the fixed-width r||s (32 bytes each).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + b64.EncodeToString(sig), nil
}

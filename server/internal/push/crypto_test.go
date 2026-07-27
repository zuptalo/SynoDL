package push

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
)

var burl = base64.RawURLEncoding

// RFC 8291 §5 test vector.
const (
	rfcPlaintext = "When I grow up, I want to be a watermelon"
	rfcAuth      = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcUAPublic  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bMwM5UkSK4rMR2u5g3T_BwuOs2E"
	rfcUAPrivate = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcASPrivate = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcSalt      = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcBody      = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func mustB(t *testing.T, s string) []byte {
	t.Helper()
	b, err := burl.DecodeString(s)
	if err != nil {
		t.Fatalf("b64 %q: %v", s, err)
	}
	return b
}

// TestSealMatchesRFCVector is the gold interop check: with the RFC's ephemeral
// key + salt, our encryptor must produce the RFC's exact message body.
func TestSealMatchesRFCVector(t *testing.T) {
	asPriv, err := ecdh.P256().NewPrivateKey(mustB(t, rfcASPrivate))
	if err != nil {
		t.Fatalf("as private: %v", err)
	}
	// Derive the receiver public key from its private key (avoids relying on a
	// separately-transcribed public-key constant).
	uaPriv, err := ecdh.P256().NewPrivateKey(mustB(t, rfcUAPrivate))
	if err != nil {
		t.Fatalf("ua private: %v", err)
	}
	body, err := seal(uaPriv.PublicKey().Bytes(), mustB(t, rfcAuth), []byte(rfcPlaintext), asPriv, mustB(t, rfcSalt))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if got := burl.EncodeToString(body); got != rfcBody {
		t.Fatalf("body mismatch:\n got  %s\n want %s", got, rfcBody)
	}
}

// TestDecryptRFCVector cross-checks our decryptor against the RFC's real output.
func TestDecryptRFCVector(t *testing.T) {
	uaPriv, err := ecdh.P256().NewPrivateKey(mustB(t, rfcUAPrivate))
	if err != nil {
		t.Fatalf("ua private: %v", err)
	}
	pt, err := decrypt(mustB(t, rfcBody), uaPriv, mustB(t, rfcAuth))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(pt) != rfcPlaintext {
		t.Fatalf("decrypt = %q, want %q", pt, rfcPlaintext)
	}
}

// TestEncryptRoundTrip: encrypt with random ephemeral+salt, then recover.
func TestEncryptRoundTrip(t *testing.T) {
	uaPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uaPubB64 := burl.EncodeToString(uaPriv.PublicKey().Bytes())
	authB64 := burl.EncodeToString([]byte("0123456789abcdef")) // 16-byte auth secret
	msg := []byte(`{"title":"done","body":"ubuntu.iso finished"}`)

	body, err := Encrypt(uaPubB64, authB64, msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := decrypt(body, uaPriv, mustB(t, authB64))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("round-trip = %q, want %q", got, msg)
	}
}

func TestVAPIDAuthHeaderVerifies(t *testing.T) {
	pub, priv, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	hdr, err := authorizationHeader("https://push.example.com/send/abc", "mailto:ops@example.com", pub, priv)
	if err != nil {
		t.Fatalf("authorizationHeader: %v", err)
	}
	// Header shape: "vapid t=<jwt>, k=<pub>".
	if !strings.HasPrefix(hdr, "vapid t=") || !strings.Contains(hdr, ", k="+pub) {
		t.Fatalf("bad header: %s", hdr)
	}
	jwt := strings.TrimPrefix(strings.SplitN(hdr, ", k=", 2)[0], "vapid t=")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts = %d", len(parts))
	}
	// Verify the ES256 signature with the VAPID public key.
	pubBytes := mustB(t, pub)
	x, y := elliptic.Unmarshal(elliptic.P256(), pubBytes) //nolint:staticcheck
	pk := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig := mustB(t, parts[2])
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pk, sum[:], r, s) {
		t.Fatal("VAPID JWT signature did not verify with the public key")
	}
	// Audience is the push endpoint's origin.
	claims := mustB(t, parts[1])
	if !strings.Contains(string(claims), `"aud":"https://push.example.com"`) {
		t.Fatalf("claims missing audience: %s", claims)
	}
}

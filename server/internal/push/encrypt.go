package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
)

// Encrypt produces the RFC 8291 "aes128gcm" Web Push message body for plaintext,
// addressed to a subscription's keys (uaPublicB64 = the client's p256dh key,
// authB64 = the auth secret, both base64url). Random ephemeral key + salt.
func Encrypt(uaPublicB64, authB64 string, plaintext []byte) ([]byte, error) {
	uaPublic, err := b64.DecodeString(uaPublicB64)
	if err != nil {
		return nil, err
	}
	authSecret, err := b64.DecodeString(authB64)
	if err != nil {
		return nil, err
	}
	return seal(uaPublic, authSecret, plaintext, nil, nil)
}

// seal is Encrypt with the ephemeral private key (asPriv) and salt injectable
// for deterministic testing against the RFC 8291 test vector; nil generates them.
func seal(uaPublic, authSecret, plaintext []byte, asPriv *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	curve := ecdh.P256()
	uaPub, err := curve.NewPublicKey(uaPublic)
	if err != nil {
		return nil, err
	}
	if asPriv == nil {
		if asPriv, err = curve.GenerateKey(rand.Reader); err != nil {
			return nil, err
		}
	}
	asPublic := asPriv.PublicKey().Bytes() // 65-byte uncompressed point
	ecdhSecret, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}
	if salt == nil {
		salt = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
	}

	cek, nonce, err := deriveKeyAndNonce(uaPublic, asPublic, authSecret, ecdhSecret, salt)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(cek)
	if err != nil {
		return nil, err
	}
	// Single (last) record: plaintext followed by the 0x02 delimiter (RFC 8188).
	record := append(append([]byte{}, plaintext...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, record, nil)

	// aes128gcm header: salt(16) || rs(4, big-endian) || idlen(1) || keyid(as_public).
	body := make([]byte, 0, 16+4+1+len(asPublic)+len(ciphertext))
	body = append(body, salt...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], 4096)
	body = append(body, rs[:]...)
	body = append(body, byte(len(asPublic)))
	body = append(body, asPublic...)
	return append(body, ciphertext...), nil
}

// deriveKeyAndNonce runs the RFC 8291 §3.4 combination followed by the RFC 8188
// aes128gcm content-encoding derivation.
func deriveKeyAndNonce(uaPublic, asPublic, authSecret, ecdhSecret, salt []byte) (cek, nonce []byte, err error) {
	// key_info = "WebPush: info" || 0x00 || ua_public || as_public
	keyInfo := append([]byte("WebPush: info\x00"), uaPublic...)
	keyInfo = append(keyInfo, asPublic...)
	prkCombine, err := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	if err != nil {
		return nil, nil, err
	}
	ikm, err := hkdf.Expand(sha256.New, prkCombine, string(keyInfo), 32)
	if err != nil {
		return nil, nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, nil, err
	}
	if cek, err = hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16); err != nil {
		return nil, nil, err
	}
	if nonce, err = hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12); err != nil {
		return nil, nil, err
	}
	return cek, nonce, nil
}

func newGCM(cek []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// decrypt reverses seal using the subscription's private key + auth secret —
// TEST/verification use only (a client/push service decrypts in production), so
// it lives here to validate the encryptor against the RFC vector and round-trip.
func decrypt(body []byte, uaPrivate *ecdh.PrivateKey, authSecret []byte) ([]byte, error) {
	if len(body) < 21 {
		return nil, errors.New("push: body too short")
	}
	salt := body[:16]
	idlen := int(body[20])
	if len(body) < 21+idlen {
		return nil, errors.New("push: bad keyid length")
	}
	asPublic := body[21 : 21+idlen]
	ciphertext := body[21+idlen:]

	asPub, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		return nil, err
	}
	ecdhSecret, err := uaPrivate.ECDH(asPub)
	if err != nil {
		return nil, err
	}
	uaPublic := uaPrivate.PublicKey().Bytes()
	cek, nonce, err := deriveKeyAndNonce(uaPublic, asPublic, authSecret, ecdhSecret, salt)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(cek)
	if err != nil {
		return nil, err
	}
	record, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	// Strip the trailing delimiter (0x02) + any 0x00 padding.
	i := len(record) - 1
	for i >= 0 && record[i] == 0x00 {
		i--
	}
	if i < 0 || record[i] != 0x02 {
		return nil, errors.New("push: bad record padding")
	}
	return record[:i], nil
}

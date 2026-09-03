// synomock runs the fake Synology DSM used by `make start` and the e2e
// harness, so neither ever needs a real NAS. See internal/synomock.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"synodl/server/internal/synomock"
)

func main() {
	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = "8291"
	}
	// DSM speaks HTTPS on its portal port, and stateful SynoDL always dials
	// https:// — so a plain-HTTP mock is unreachable from the stateful path, which
	// is where accounts and the download sources live. MOCK_TLS makes the mock
	// serve TLS with a throwaway self-signed certificate, exactly the shape of a
	// real self-signed NAS (which the operator meets via SYNO_TLS_INSECURE).
	//
	// Off by default so the plain-HTTP callers (the e2e harness, hand testing with
	// curl) are unaffected; `make start` turns it on.
	handler := synomock.New().Handler()
	if os.Getenv("MOCK_TLS") == "1" {
		cert, err := selfSignedCert()
		if err != nil {
			slog.Error("generate mock certificate", "err", err)
			os.Exit(1)
		}
		slog.Info("synomock (fake DSM) starting", "port", port, "tls", true,
			"accounts", "admin/secret, otpuser/secret+OTP 000000, disabled/blocked/expired (guide states)")
		srv := &http.Server{
			Addr:      ":" + port,
			Handler:   handler,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		}
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
		return
	}
	slog.Info("synomock (fake DSM) starting", "port", port, "tls", false,
		"accounts", "admin/secret, otpuser/secret+OTP 000000, disabled/blocked/expired (guide states)")
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

// selfSignedCert mints a short-lived certificate for localhost, in memory. It is
// never written to disk and never reused across runs: this is a fake NAS for
// development, and a long-lived key sitting in the repo would be a liability for
// no benefit.
func selfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "synomock"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

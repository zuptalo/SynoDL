//go:build sourcemock

package people

import (
	"crypto/tls"
	"os"
	"strings"
)

// mockIMDbBase redirects the photograph lookup at a fake IMDb, for dev and e2e
// only.
//
// Compiled ONLY under the `sourcemock` build tag, exactly like the download
// drivers' mockBase — see mockbase_prod.go. A release binary has no such branch,
// so there is no environment variable, config key or admin setting that could
// point this feature anywhere but IMDb.
//
//	IMDB_MOCK_BASE=http://localhost:8291/mockimdb
func mockIMDbBase() string {
	return strings.TrimRight(os.Getenv("IMDB_MOCK_BASE"), "/")
}

// outboundTLS for dev and e2e builds. The in-repo fake IMDb is served by the
// mock, which presents a self-signed certificate it mints per run — exactly as
// the fake NAS and the fake source sites do — so a verifying client cannot talk
// to it. Mirrors internal/source/tlsconfig_dev.go.
func outboundTLS() *tls.Config { return &tls.Config{InsecureSkipVerify: true} } //nolint:gosec // dev/e2e builds only

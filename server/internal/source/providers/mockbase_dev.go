//go:build sourcemock

package providers

import (
	"os"
	"strings"
)

// mockBase redirects a driver at a fake site, for dev and e2e only.
//
// This file is compiled ONLY under the `sourcemock` build tag. A release binary
// is built without it, so the redirect cannot exist in production however the
// environment is set — see mockbase_prod.go.
//
//	SOURCE_MOCK_ZARFILM=http://localhost:8291/mocksrc/zar
//	SOURCE_MOCK_30NAMA=http://localhost:8291/mocksrc/tn
func mockBase(kind string) string {
	return strings.TrimRight(os.Getenv("SOURCE_MOCK_"+strings.ToUpper(kind)), "/")
}

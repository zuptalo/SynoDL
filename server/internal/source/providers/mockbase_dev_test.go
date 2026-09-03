//go:build sourcemock

package providers

import (
	"strings"
	"testing"

	"synodl/server/internal/source"
)

// The other half of the contract: under the dev/e2e build tag the redirect works
// and brings the fake site's host into the allowlist with it — otherwise the
// driver would be pointed at a host it then refuses to call.
func TestMockBaseRedirectsUnderDevTag(t *testing.T) {
	t.Setenv("SOURCE_MOCK_ZARFILM", "http://localhost:8291/mocksrc/zar")
	drv := zarfilm{}
	if got := drv.base(); got != "http://localhost:8291/mocksrc/zar" {
		t.Fatalf("base = %q", got)
	}
	cfg := drv.Hosts()
	if !source.HostAllowed("localhost", cfg.APIHosts) {
		t.Fatalf("fake host not allowlisted: %v", cfg.APIHosts)
	}
	// The real hosts must still be allowed — a dev build is not a different
	// driver, just one that can additionally reach a fake.
	if !source.HostAllowed(zarHost, cfg.APIHosts) {
		t.Fatalf("real host lost from allowlist: %v", cfg.APIHosts)
	}
	if !strings.Contains(strings.Join(cfg.DownloadHosts, ","), zarDownload) {
		t.Fatalf("real download host lost: %v", cfg.DownloadHosts)
	}
}

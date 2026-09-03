//go:build !sourcemock

package providers

import "testing"

// FR-025 / the credential-safety checklist: redirecting a driver at a fake site
// must be impossible in a production build. It is a build-tag capability, so in
// an ordinary build (no `sourcemock` tag — which is how this test runs, and how
// releases are built) no environment variable can turn it on.
//
// If someone ever converts this into a runtime flag, this test fails and says
// why.
func TestMockBaseIsUnavailableInAProductionBuild(t *testing.T) {
	t.Setenv("SOURCE_MOCK_ZARFILM", "http://attacker.example/mocksrc/zar")
	t.Setenv("SOURCE_MOCK_30NAMA", "http://attacker.example/mocksrc/tn")

	if got := mockBase("zarfilm"); got != "" {
		t.Fatalf("an environment variable redirected a driver in a production build: %q", got)
	}
	// And the allowlist must not have grown a host from it.
	cfg := zarfilm{}.Hosts()
	for _, h := range append(append([]string{}, cfg.APIHosts...), cfg.DownloadHosts...) {
		if h == "attacker.example" {
			t.Fatal("the outbound allowlist absorbed a host from the environment")
		}
	}
	drv := zarfilm{}
	if got := drv.base(); got != zarBase {
		t.Fatalf("driver base was redirected: %q", got)
	}
}

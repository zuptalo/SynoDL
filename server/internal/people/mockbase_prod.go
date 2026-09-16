//go:build !sourcemock

package people

import "crypto/tls"

// Pointing the photograph lookup at a fake IMDb is a BUILD-TIME capability, not
// a runtime one. In an ordinary build this is the only definition that exists
// and it always returns "", so the two-host allowlist above is structural — as
// the constitution requires of every outbound surface.
func mockIMDbBase() string { return "" }

// outboundTLS is the TLS configuration for a photograph lookup: full
// verification, always. In an ordinary build this is the only definition, so
// there is no switch — environment, config or otherwise — that could weaken
// certificate checking on the way to IMDb.
func outboundTLS() *tls.Config { return &tls.Config{} }

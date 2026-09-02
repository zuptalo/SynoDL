//go:build sourcemock

package source

import "crypto/tls"

// outboundTLS for dev and e2e builds. The in-repo fake source site presents a
// self-signed certificate it mints per run, exactly as the fake NAS does, so a
// verifying client cannot talk to it.
//
// This file is compiled ONLY under the `sourcemock` build tag, which a release
// build never passes — so production keeps full verification with no way to
// turn it off. It mirrors the NAS side, where a self-signed certificate is an
// explicit opt-in rather than a silent default.
func outboundTLS() *tls.Config { return &tls.Config{InsecureSkipVerify: true} } //nolint:gosec // dev/e2e builds only

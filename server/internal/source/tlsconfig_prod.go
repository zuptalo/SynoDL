//go:build !sourcemock

package source

import "crypto/tls"

// outboundTLS is the TLS configuration for provider calls: full verification,
// always. In an ordinary build this is the only definition, so there is no
// switch — environment, config or otherwise — that could weaken certificate
// checking for an outbound source request.
func outboundTLS() *tls.Config { return &tls.Config{} }

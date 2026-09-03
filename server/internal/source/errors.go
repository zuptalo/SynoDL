package source

import (
	"errors"
	"fmt"
)

// Expiry layers reported with ErrNeedsRefresh. Best-effort: the client can map
// all of them to a single "source needs refreshing" user state.
const (
	LayerClearance = "clearance" // bot-protection (cf_clearance) rejected
	LayerToken     = "token"     // API auth (c-token) rejected
	LayerIP        = "ip"        // request/link bound to a different public IP
)

// ErrNeedsRefresh means the stored provider session no longer works and an admin
// must re-paste fresh material. It carries a best-effort Layer but never any
// secret value or raw upstream body.
type ErrNeedsRefresh struct {
	Layer string
}

func (e *ErrNeedsRefresh) Error() string {
	return fmt.Sprintf("source session needs refresh (%s)", e.Layer)
}

// AsNeedsRefresh extracts an *ErrNeedsRefresh from an error chain.
func AsNeedsRefresh(err error) (*ErrNeedsRefresh, bool) {
	var e *ErrNeedsRefresh
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// ErrUnsubscribed means the session is valid but the account has no download
// entitlement. Kept distinct from ErrNeedsRefresh on purpose: telling an operator
// to re-paste a session that is working perfectly sends them in circles.
var ErrUnsubscribed = errors.New("source: account has no download entitlement")

// ErrUnavailable marks a failure that means "this address did not answer" —
// connection refused, DNS failure, timeout, or a server-side error. It is the
// ONLY condition that justifies falling back to a source's alternate domain
// (spec 1020 FR-003/FR-004): an authentication failure is not an outage, and
// retrying it elsewhere would fail identically while hiding the real cause.
type ErrUnavailable struct {
	Err error
}

func (e *ErrUnavailable) Error() string { return "source: address unavailable: " + e.Err.Error() }
func (e *ErrUnavailable) Unwrap() error { return e.Err }

// IsUnavailable reports whether err is an availability failure.
func IsUnavailable(err error) bool {
	var e *ErrUnavailable
	return errors.As(err, &e)
}

// ErrHostNotAllowed is returned when an outbound request targets a host outside
// the provider's configured allowlist. It must never reach a client verbatim
// (it names an internal host), but it is safe to log.
var ErrHostNotAllowed = errors.New("source: outbound host not in provider allowlist")

// ErrProviderVerify categorizes a failed configure/refresh verification without
// echoing upstream detail. Reason is one of: "challenge", "invalid_token",
// "ip_mismatch", "unreachable".
type ErrProviderVerify struct {
	Reason string
}

func (e *ErrProviderVerify) Error() string { return "source: verify failed: " + e.Reason }

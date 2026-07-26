package syno

import (
	"errors"
	"fmt"
)

// Kind classifies a DSM failure into the categories the client UI acts on.
// The mapping happens once, here, so handlers and the PWA never interpret raw
// DSM error codes.
type Kind string

const (
	// KindSession — the sid is missing/expired/revoked; the client must log in again.
	KindSession Kind = "session"
	// KindCredentials — wrong account or password.
	KindCredentials Kind = "credentials"
	// KindOTPRequired — the account has 2FA enabled and no code was given.
	KindOTPRequired Kind = "otp_required"
	// KindOTPInvalid — the 2FA code was wrong or expired.
	KindOTPInvalid Kind = "otp_invalid"
	// KindPermission — the account lacks Download Station / FileStation privileges.
	KindPermission Kind = "permission"
	// KindUnreachable — the NAS could not be reached (network/TLS/timeout).
	KindUnreachable Kind = "nas_unreachable"
	// KindNAS — any other DSM-reported error.
	KindNAS Kind = "nas"
)

// Error is a typed DSM failure carrying the classification plus the raw code
// for diagnostics. It deliberately contains no request parameters — never any
// credential, sid, or URI — so it is safe to log verbatim (Principle III).
type Error struct {
	Kind Kind
	Code int    // raw DSM error code (0 for transport failures)
	API  string // DSM API that failed, e.g. "SYNO.API.Auth"
}

func (e *Error) Error() string {
	if e.Code == 0 {
		return fmt.Sprintf("syno: %s (%s)", e.Kind, e.API)
	}
	return fmt.Sprintf("syno: %s (%s code %d)", e.Kind, e.API, e.Code)
}

// AsError extracts a *Error from err, or nil.
func AsError(err error) *Error {
	var se *Error
	if errors.As(err, &se) {
		return se
	}
	return nil
}

// classify maps a DSM error code from the given API to its Kind.
//
// Common codes (all APIs): 105 privilege, 106/107 session timeout/duplicate
// login, 119 sid not found. SYNO.API.Auth adds: 400 bad account/password,
// 401 disabled account, 402 permission, 403 2FA code required, 404 2FA code
// failed (DSM7 also uses 403/404 this way; DSM6 semantics match).
func classify(api string, code int) Kind {
	switch code {
	case 106, 107, 119:
		return KindSession
	case 105:
		// On auth.cgi 105 means "insufficient privilege" for the app session;
		// elsewhere it means the sid lost its privilege — either way the client
		// can only recover by logging in again with a capable account.
		if api == apiAuth {
			return KindPermission
		}
		return KindSession
	}
	if api == apiAuth {
		switch code {
		case 400:
			return KindCredentials
		case 401, 402:
			return KindPermission
		case 403:
			return KindOTPRequired
		case 404:
			return KindOTPInvalid
		}
	}
	return KindNAS
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// handleGetNASConfig returns the stored operator connection settings for the
// admin Settings screen — the NON-secret projection only. The NAS password
// never leaves the server (Principle III), so it is omitted entirely; the client
// gets a `nasUses2FA` flag and a blank password field to re-enter on change.
func handleGetNASConfig(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, _ *http.Request, _ *store.User) {
		c, err := d.Store.GetOperatorConfig()
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpx.Error(w, http.StatusConflict, "setup_required")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"publicUrl":    c.PublicURL,
			"nasAddress":   c.NASAddress,
			"nasPort":      c.NASPort,
			"nasTlsVerify": c.NASTLSVerify,
			"nasAccount":   c.NASAccount,
			"nasUses2FA":   c.NASUses2FA,
		})
	})
}

// connReq is the shared shape for testing and updating the NAS connection.
// A blank password means "use the stored one" (so the admin can re-test or edit
// non-secret fields without re-typing the NAS password).
type connReq struct {
	PublicURL    *string `json:"publicUrl"`
	NASAddress   string  `json:"nasAddress"`
	NASPort      int     `json:"nasPort"`
	NASTLSVerify bool    `json:"nasTlsVerify"`
	NASAccount   string  `json:"nasAccount"`
	NASPassword  string  `json:"nasPassword"`
	OTP          string  `json:"otp"`
}

// handleTestNASConnection verifies a candidate connection by establishing (and
// immediately dropping) a real DSM session — nothing is persisted. Lets the
// admin confirm changes before saving. Blank fields fall back to the stored
// config so an unchanged-password re-test works.
func handleTestNASConnection(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body connReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		body.NASAddress = strings.TrimSpace(body.NASAddress)
		body.NASAccount = strings.TrimSpace(body.NASAccount)

		// Fill blanks from the stored config so a partial edit can still be tested.
		if cur, err := d.Store.GetOperatorConfig(); err == nil {
			if body.NASAddress == "" {
				body.NASAddress = cur.NASAddress
			}
			if body.NASPort == 0 {
				body.NASPort = cur.NASPort
			}
			if body.NASAccount == "" {
				body.NASAccount = cur.NASAccount
			}
			if body.NASPassword == "" {
				body.NASPassword = cur.NASPassword
			}
		}
		if body.NASAddress == "" || body.NASPort <= 0 || body.NASAccount == "" || body.NASPassword == "" {
			httpx.Error(w, http.StatusBadRequest, "missing or invalid fields")
			return
		}
		if err := d.NAS.VerifyLogin(r.Context(), body.NASAddress, body.NASPort, body.NASTLSVerify,
			body.NASAccount, body.NASPassword, body.OTP); err != nil {
			writeSynoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// handleUpdateNASConfig edits the stored connection. Non-connection edits (just
// the public URL) save without touching the NAS. A connection/credential change
// re-verifies by establishing a session and rolls back to the previous config
// if the NAS rejects it, so a bad edit never breaks the running instance.
func handleUpdateNASConfig(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		cur, err := d.Store.GetOperatorConfig()
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpx.Error(w, http.StatusConflict, "setup_required")
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		var body connReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}

		next := *cur // start from the stored config; apply only what's provided
		if body.PublicURL != nil {
			next.PublicURL = strings.TrimSpace(*body.PublicURL)
		}
		if a := strings.TrimSpace(body.NASAddress); a != "" {
			next.NASAddress = a
		}
		if body.NASPort > 0 {
			next.NASPort = body.NASPort
		}
		if acc := strings.TrimSpace(body.NASAccount); acc != "" {
			next.NASAccount = acc
		}
		passwordChanged := body.NASPassword != "" && body.NASPassword != cur.NASPassword
		if body.NASPassword != "" {
			next.NASPassword = body.NASPassword
		}
		// TLS/OTP always reflect this request's connection intent.
		next.NASTLSVerify = body.NASTLSVerify
		// 2FA follows a fresh OTP; a password change without an OTP asserts no 2FA;
		// otherwise keep whatever the setup established.
		switch {
		case body.OTP != "":
			next.NASUses2FA = true
		case passwordChanged:
			next.NASUses2FA = false
		default:
			next.NASUses2FA = cur.NASUses2FA
		}

		connectionChanged := next.NASAddress != cur.NASAddress || next.NASPort != cur.NASPort ||
			next.NASTLSVerify != cur.NASTLSVerify || next.NASAccount != cur.NASAccount ||
			passwordChanged || body.OTP != ""

		if err := d.Store.SaveOperatorConfig(next); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if !connectionChanged {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Re-verify the new connection; restore the previous config on failure so a
		// rejected edit leaves the instance working, not broken.
		d.NAS.Invalidate()
		var verr error
		if next.NASUses2FA {
			verr = d.NAS.Reauth(r.Context(), body.OTP)
		} else {
			_, verr = d.NAS.SID(r.Context())
		}
		if verr != nil {
			_ = d.Store.SaveOperatorConfig(*cur)
			d.NAS.Invalidate()
			writeNASError(w, verr)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

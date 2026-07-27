package api

import (
	"encoding/json"
	"net/http"

	"synodl/server/internal/auth"
	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// handleSynoDLLogin authenticates a SynoDL user (NOT the NAS) and issues a
// session. The response is uniform on any failure so it never reveals whether
// the username exists, the password was wrong, or the account is disabled.
func handleSynoDLLogin(d Deps) http.Handler {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		u, err := d.Store.GetUserByUsername(body.Username)
		if err != nil || !u.IsEnabled || !auth.VerifyPassword(u.PasswordHash, body.Password) {
			httpx.Error(w, http.StatusUnauthorized, "credentials")
			return
		}
		d.issueSession(w, u)
	})
}

// handleSynoDLLogout drops the caller's session (best-effort; the client clears
// its token regardless).
func handleSynoDLLogout(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.Header.Get(sessionHeader); tok != "" {
			_ = d.Store.DeleteSession(auth.HashToken(tok))
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// handleMe returns the signed-in user (used by the client to restore state and
// gate admin UI).
func handleMe(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		httpx.JSON(w, http.StatusOK, userView(u))
	})
}

// handleNASReauth lets an admin supply a fresh 2FA code to restore the shared
// NAS session after it expires (background re-auth can't supply a TOTP code).
func handleNASReauth(d Deps) http.Handler {
	type req struct {
		OTP string `json:"otp"`
	}
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := d.NAS.Reauth(r.Context(), body.OTP); err != nil {
			writeNASError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

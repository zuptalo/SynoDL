package api

import (
	"encoding/json"
	"net/http"

	"synodl/server/internal/httpx"
)

// handleLogin forwards the DSM login and hands the resulting sid straight back
// to the client. The password exists only inside this request (Principle III):
// it is never stored, and the decoded body is not logged anywhere.
func handleLogin(d Deps) http.Handler {
	type loginReq struct {
		Account  string `json:"account"`
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if req.Account == "" || req.Password == "" {
			httpx.Error(w, http.StatusBadRequest, "account and password required")
			return
		}
		sid, err := d.Syno.Login(r.Context(), req.Account, req.Password, req.OTP)
		if err != nil {
			writeSynoError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{
			"sid":     sid,
			"account": req.Account,
		})
	})
}

// handleLogout invalidates the NAS session. Best-effort by design: the client
// drops its sid regardless, so a NAS error here still returns 204 — there is
// no server-side session to clean up (Principle III).
func handleLogout(d Deps) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		_ = d.Syno.Logout(r.Context(), sid)
		w.WriteHeader(http.StatusNoContent)
	})
}

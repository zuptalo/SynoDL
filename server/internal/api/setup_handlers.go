package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"synodl/server/internal/auth"
	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// handleSetupState reports whether first-run setup is complete. Before it is, it
// offers a NAS-URL prefill from any legacy SYNO_URL so the operator isn't
// retyping known details. Public (the wizard runs before any account exists).
func handleSetupState(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configured, err := d.Store.HasOperatorConfig()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		resp := map[string]any{"configured": configured}
		if !configured && d.Cfg.SynoURL != "" {
			resp["prefillNasUrl"] = d.Cfg.SynoURL
		}
		httpx.JSON(w, http.StatusOK, resp)
	})
}

// handleSetupSubmit performs first-run setup: verify the NAS login by
// establishing a real session, store the connection + the admin account, and
// sign the admin in. It runs exactly once — a second attempt after setup is
// rejected so the stored connection can't be silently overwritten.
func handleSetupSubmit(d Deps) http.Handler {
	type req struct {
		PublicURL     string `json:"publicUrl"`
		NASAddress    string `json:"nasAddress"`
		NASPort       int    `json:"nasPort"`
		NASTLSVerify  bool   `json:"nasTlsVerify"`
		NASAccount    string `json:"nasAccount"`
		NASPassword   string `json:"nasPassword"`
		OTP           string `json:"otp"`
		AdminUsername string `json:"adminUsername"`
		AdminPassword string `json:"adminPassword"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configured, err := d.Store.HasOperatorConfig()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if configured {
			httpx.Error(w, http.StatusConflict, "already_configured")
			return
		}
		var body req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		body.NASAddress = strings.TrimSpace(body.NASAddress)
		body.NASAccount = strings.TrimSpace(body.NASAccount)
		body.AdminUsername = strings.TrimSpace(body.AdminUsername)
		if body.NASAddress == "" || body.NASPort <= 0 || body.NASAccount == "" ||
			body.NASPassword == "" || body.AdminUsername == "" || len(body.AdminPassword) < 8 {
			httpx.Error(w, http.StatusBadRequest, "missing or invalid fields")
			return
		}
		uses2FA := body.OTP != ""

		// Store tentatively so the connection manager builds a client for these
		// details, then verify by establishing a session; roll back if the NAS
		// rejects us, so a failed setup leaves the wizard runnable, not a broken
		// stored connection.
		if err := d.Store.SaveOperatorConfig(store.OperatorConfig{
			PublicURL: body.PublicURL, NASAddress: body.NASAddress, NASPort: body.NASPort,
			NASTLSVerify: body.NASTLSVerify, NASAccount: body.NASAccount,
			NASPassword: body.NASPassword, NASUses2FA: uses2FA,
		}); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		d.NAS.Invalidate()
		var verr error
		if uses2FA {
			verr = d.NAS.Reauth(r.Context(), body.OTP)
		} else {
			_, verr = d.NAS.SID(r.Context())
		}
		if verr != nil {
			_ = d.Store.DeleteOperatorConfig()
			writeSynoError(w, verr)
			return
		}

		hash, err := auth.HashPassword(body.AdminPassword)
		if err != nil {
			_ = d.Store.DeleteOperatorConfig()
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		uid, err := d.Store.CreateUser(body.AdminUsername, hash, true)
		if err != nil {
			_ = d.Store.DeleteOperatorConfig()
			httpx.Error(w, http.StatusBadRequest, "could not create admin account")
			return
		}
		u, err := d.Store.GetUserByID(uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		d.issueSession(w, u)
	})
}

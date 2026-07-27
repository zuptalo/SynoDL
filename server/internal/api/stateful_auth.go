package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"synodl/server/internal/auth"
	"synodl/server/internal/httpx"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
)

// sessionHeader carries the SynoDL session token. Like the old X-Syno-Sid, it is
// a header (not a cookie): the client owns it, so CSRF is structurally
// impossible and there are no cookie scopes to manage.
const sessionHeader = "X-SynoDL-Session"

// sessionTTL is how long a SynoDL sign-in lasts before re-login is required.
const sessionTTL = 30 * 24 * time.Hour

type ctxKey int

const userCtxKey ctxKey = iota

func userFromContext(r *http.Request) *store.User {
	u, _ := r.Context().Value(userCtxKey).(*store.User)
	return u
}

// requireUser resolves the SynoDL session token to a live, enabled user, or
// rejects with 401 "session".
func (d Deps) requireUser(next func(http.ResponseWriter, *http.Request, *store.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get(sessionHeader)
		if tok == "" {
			httpx.Error(w, http.StatusUnauthorized, "session")
			return
		}
		u, err := d.Store.UserForSession(auth.HashToken(tok), time.Now().Unix())
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusUnauthorized, "session")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, u)), u)
	})
}

// requireAdmin is requireUser plus an admin check (enforced server-side, not
// merely hidden in the UI).
func (d Deps) requireAdmin(next func(http.ResponseWriter, *http.Request, *store.User)) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if !u.IsAdmin {
			httpx.Error(w, http.StatusForbidden, "permission")
			return
		}
		next(w, r, u)
	})
}

// issueSession mints a session for the user and writes the sign-in response
// (token + a non-secret user view). The raw token is returned once; only its
// hash is stored.
func (d Deps) issueSession(w http.ResponseWriter, u *store.User) {
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}
	if err := d.Store.CreateSession(hash, u.ID, time.Now().Add(sessionTTL).Unix()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"token": token, "user": userView(u)})
}

func userView(u *store.User) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "isAdmin": u.IsAdmin}
}

// writeNASError maps NAS connection-manager failures to the client contract:
// not-configured ⇒ 409 setup_required, needs-2FA ⇒ 503 nas_reauth, else the
// usual DSM error mapping.
func writeNASError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, nas.ErrNotConfigured):
		httpx.Error(w, http.StatusConflict, "setup_required")
	case errors.Is(err, nas.ErrNeedsReauth):
		httpx.Error(w, http.StatusServiceUnavailable, "nas_reauth")
	default:
		writeSynoError(w, err)
	}
}

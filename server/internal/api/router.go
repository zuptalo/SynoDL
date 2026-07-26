// Package api wires the HTTP surface: typed /v1 endpoints in front of the
// syno.Client allowlist, the health probe, and (in production) the static PWA.
package api

import (
	"net/http"

	"synodl/server/internal/config"
	"synodl/server/internal/httpx"
	"synodl/server/internal/syno"
)

// Deps carries everything the router needs. Handlers depend on the small
// syno.Client interface so tests can pass a fake.
type Deps struct {
	Cfg          config.Config
	Syno         syno.Client
	Version      string
	ReleaseNotes []ReleaseNote
}

// NewRouter builds the full handler tree with the recover → log → CORS
// middleware chain, mirroring the order used across the codebase: recovery
// outermost so even logging panics turn into a 500 line.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("GET /v1/config", handleConfig(d))

	// Login is the one endpoint an unauthenticated caller can drive against the
	// NAS, so it alone is rate-limited per client IP (Principle III: the proxy
	// must not become a brute-force amplifier).
	limiter := httpx.NewLimiter(d.Cfg.LoginPerMinute)
	mux.Handle("POST /v1/session", limiter.Middleware(handleLogin(d)))
	mux.Handle("DELETE /v1/session", handleLogout(d))

	mux.Handle("GET /v1/tasks", handleListTasks(d))
	mux.Handle("POST /v1/tasks", handleCreateTask(d))
	mux.Handle("POST /v1/tasks/pause", handleTaskAction(d, "pause"))
	mux.Handle("POST /v1/tasks/resume", handleTaskAction(d, "resume"))
	mux.Handle("POST /v1/tasks/delete", handleTaskAction(d, "delete"))

	mux.Handle("GET /v1/fs/shares", handleListShares(d))
	mux.Handle("GET /v1/fs/list", handleListFolder(d))

	// Static serving is mounted last so /v1 + /healthz stay authoritative:
	// spaHandler/devProxyHandler both 404 API paths instead of shadowing them.
	if d.Cfg.DevProxy != "" && d.Cfg.Env == "dev" {
		mux.Handle("/", devProxyHandler(d.Cfg.DevProxy))
	} else if d.Cfg.StaticDir != "" {
		mux.Handle("/", spaHandler(d.Cfg.StaticDir))
	}

	return httpx.Chain(mux, httpx.Recover, httpx.Log, httpx.CORS(d.Cfg.AllowedOrigins))
}

// sid extracts the client's NAS session id. The sid travels in a header (not a
// cookie) so cross-site request forgery is structurally impossible and the
// server never has to manage cookie scopes — the client owns the session
// (Principle III).
func sid(r *http.Request) string {
	return r.Header.Get("X-Syno-Sid")
}

// requireSid guards a handler behind a present (not necessarily still valid —
// the NAS decides that) session header, so anonymous requests never reach the
// NAS at all.
func requireSid(next func(w http.ResponseWriter, r *http.Request, sid string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := sid(r)
		if s == "" {
			httpx.Error(w, http.StatusUnauthorized, "session")
			return
		}
		next(w, r, s)
	})
}

// writeSynoError translates a syno.Client failure into the HTTP + error-string
// contract the PWA acts on. All session-shaped failures become 401 "session"
// (etc.) so the client's routing logic stays a simple switch on one string.
func writeSynoError(w http.ResponseWriter, err error) {
	se := syno.AsError(err)
	if se == nil {
		httpx.Error(w, http.StatusBadGateway, string(syno.KindNAS))
		return
	}
	switch se.Kind {
	case syno.KindSession, syno.KindCredentials, syno.KindOTPRequired, syno.KindOTPInvalid:
		httpx.Error(w, http.StatusUnauthorized, string(se.Kind))
	case syno.KindPermission:
		httpx.Error(w, http.StatusForbidden, string(se.Kind))
	case syno.KindUnreachable:
		httpx.Error(w, http.StatusBadGateway, string(se.Kind))
	default:
		httpx.Error(w, http.StatusBadGateway, string(syno.KindNAS))
	}
}

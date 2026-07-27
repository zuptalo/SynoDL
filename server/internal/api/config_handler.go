package api

import (
	"net/http"
	"net/url"

	"synodl/server/internal/httpx"
)

// ReleaseNote is one "What's new" line, stamped into the binary at build time
// (main.releaseNotesB64) and surfaced by the update prompt.
type ReleaseNote struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// handleConfig exposes the little the client needs before login: the app
// version, release notes for the update prompt, and the NAS host for the
// Settings "Host" row. Host only — never the full URL, port, or credentials.
func handleConfig(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"version":      d.Version,
			"releaseNotes": d.ReleaseNotes,
			"nasHost":      nasHostFor(d),
		})
	})
}

// nasHostFor returns the bare NAS hostname for the Settings screen. In stateful
// mode the source of truth is the wizard-configured connection in the store
// (so editing it in Settings takes effect), not the SYNO_URL env — which in
// stateful mode is only a first-run prefill. Legacy mode parses SYNO_URL.
func nasHostFor(d Deps) string {
	if d.Stateful && d.Store != nil {
		if c, err := d.Store.GetOperatorConfig(); err == nil {
			return c.NASAddress // already a bare host — no scheme/port
		}
		return ""
	}
	if u, err := url.Parse(d.Cfg.SynoURL); err == nil {
		return u.Hostname()
	}
	return ""
}

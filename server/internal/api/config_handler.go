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
	nasHost := ""
	if u, err := url.Parse(d.Cfg.SynoURL); err == nil {
		nasHost = u.Hostname()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"version":      d.Version,
			"releaseNotes": d.ReleaseNotes,
			"nasHost":      nasHost,
		})
	})
}

package api

import (
	"net/http"
	"strings"

	"synodl/server/internal/httpx"
)

// handleListShares lists the NAS's shared folders — the top level of the
// destination folder picker.
func handleListShares(d Deps) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		folders, err := d.Syno.ListShares(r.Context(), sid)
		if err != nil {
			writeSynoError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

// handleListFolder drills into one folder (dirs only). The path must be
// rooted — the NAS enforces its own permissions, but rejecting junk here keeps
// error handling honest.
func handleListFolder(d Deps) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		path := r.URL.Query().Get("path")
		if !strings.HasPrefix(path, "/") {
			httpx.Error(w, http.StatusBadRequest, "path must be absolute")
			return
		}
		folders, err := d.Syno.ListFolder(r.Context(), sid, path)
		if err != nil {
			writeSynoError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

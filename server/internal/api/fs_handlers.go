package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"synodl/server/internal/httpx"
)

// createFolderReq is the shared body for POST /v1/fs/folder: create `name`
// under the absolute parent `path`.
type createFolderReq struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// validFolderName rejects empty names, dot entries, and any path separators so
// a "new folder" can only ever be one level under the given parent.
func validFolderName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

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

// handleCreateFolder creates a subfolder under an absolute parent path (the NAS
// enforces its own permissions in this legacy path).
func handleCreateFolder(d Deps) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		var body createFolderReq
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		body.Path = strings.TrimSpace(body.Path)
		body.Name = strings.TrimSpace(body.Name)
		if !strings.HasPrefix(body.Path, "/") || !validFolderName(body.Name) {
			httpx.Error(w, http.StatusBadRequest, "absolute path and a valid folder name required")
			return
		}
		folder, err := d.Syno.CreateFolder(r.Context(), sid, body.Path, body.Name)
		if err != nil {
			writeSynoError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folder": folder})
	})
}

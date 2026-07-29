package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"synodl/server/internal/authz"
	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// grantsFor loads a user's folder grants (empty for admins, who are unrestricted).
func (d Deps) grantsFor(u *store.User) []string {
	if u.IsAdmin {
		return nil
	}
	g, err := d.Store.ListFolderGrants(u.ID)
	if err != nil {
		return nil
	}
	return g
}

// destinationAllowed reports whether a user may create a task into dest. Admins
// may use the NAS default (empty) or any folder; a non-admin must name a folder
// within their grants (empty is rejected so a scoped user can't fall back to the
// NAS default).
func (d Deps) destinationAllowed(u *store.User, dest string) bool {
	if u.IsAdmin {
		return dest == "" || authz.AllowedForCreate(true, nil, dest)
	}
	if dest == "" {
		return false
	}
	return authz.AllowedForCreate(false, d.grantsFor(u), dest)
}

// filterFolders keeps only folders the user may see in the destination picker.
func (d Deps) filterFolders(u *store.User, folders []syno.Folder) []syno.Folder {
	if u.IsAdmin {
		return folders
	}
	grants := d.grantsFor(u)
	out := make([]syno.Folder, 0, len(folders))
	for _, f := range folders {
		if authz.VisibleInPicker(false, grants, f.Path) {
			out = append(out, f)
		}
	}
	return out
}

// These mirror the legacy task/fs handlers but authenticate the caller as a
// SynoDL user and reach the NAS through the shared connection manager (which
// owns the DSM session) instead of a client-supplied sid. Folder-scope
// enforcement is added in Increment 3.

func handleListTasksStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var tasks []syno.Task
		var stats syno.Stats
		err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			t, e := c.ListTasks(r.Context(), sid)
			if e != nil {
				return e
			}
			tasks = t
			stats, _ = c.Stats(r.Context(), sid) // stats failure degrades to zeros
			return nil
		})
		if err != nil {
			writeNASError(w, err)
			return
		}
		// Per-user ownership: hide other people's tasks in "own" scope and label
		// each remaining row "added by <user>" for admins.
		views := d.decorateTasks(u, tasks)
		if views == nil {
			views = []taskView{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"tasks": views, "stats": stats})
	})
}

func handleCreateTaskStateful(d Deps) http.Handler {
	maxBytes := int64(d.Cfg.MaxTorrentMB) << 20
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			if err := r.ParseMultipartForm(maxBytes); err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					httpx.Error(w, http.StatusRequestEntityTooLarge, "torrent file too large")
					return
				}
				httpx.Error(w, http.StatusBadRequest, "bad multipart body")
				return
			}
			file, header, err := r.FormFile("torrent")
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "torrent file missing")
				return
			}
			defer file.Close()
			opts := syno.CreateOpts{Destination: r.FormValue("destination"), UnzipPassword: r.FormValue("unzipPassword")}
			if !d.destinationAllowed(u, opts.Destination) {
				httpx.Error(w, http.StatusForbidden, "folder_denied")
				return
			}
			if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
				return c.CreateTaskFile(r.Context(), sid, header.Filename, file, opts)
			}); err != nil {
				writeNASError(w, err)
				return
			}
			// Record who created it so the watcher can attribute notifications.
			_ = d.Store.AddTaskClaim(u.ID, header.Filename, time.Now().Unix())
			w.WriteHeader(http.StatusCreated)
			return
		}

		var req createTaskJSON
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		uris := req.URIs[:0]
		for _, u := range req.URIs {
			if u = strings.TrimSpace(u); u != "" {
				uris = append(uris, u)
			}
		}
		if len(uris) == 0 {
			httpx.Error(w, http.StatusBadRequest, "at least one URL required")
			return
		}
		opts := syno.CreateOpts{
			Destination: req.Destination, Username: req.Username,
			Password: req.Password, UnzipPassword: req.UnzipPassword,
		}
		if !d.destinationAllowed(u, opts.Destination) {
			httpx.Error(w, http.StatusForbidden, "folder_denied")
			return
		}
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			return c.CreateTaskURIs(r.Context(), sid, uris, opts)
		}); err != nil {
			writeNASError(w, err)
			return
		}
		// Record who created each task so the watcher can attribute notifications.
		now := time.Now().Unix()
		for _, uri := range uris {
			_ = d.Store.AddTaskClaim(u.ID, titleHint(uri), now)
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func handleTaskActionStateful(d Deps, action string) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		if len(req.IDs) == 0 {
			httpx.Error(w, http.StatusBadRequest, "task ids required")
			return
		}
		err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			switch action {
			case "pause":
				return c.PauseTasks(r.Context(), sid, req.IDs)
			case "resume":
				return c.ResumeTasks(r.Context(), sid, req.IDs)
			default:
				return c.DeleteTasks(r.Context(), sid, req.IDs)
			}
		})
		if err != nil {
			writeNASError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func handleListSharesStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var folders []syno.Folder
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			f, e := c.ListShares(r.Context(), sid)
			folders = f
			return e
		}); err != nil {
			writeNASError(w, err)
			return
		}
		folders = d.filterFolders(u, folders) // only shares within/leading to the user's grants
		if folders == nil {
			folders = []syno.Folder{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

func handleListFolderStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		path := r.URL.Query().Get("path")
		if path == "" {
			httpx.Error(w, http.StatusBadRequest, "path required")
			return
		}
		// A non-admin may only browse into folders their grants make visible.
		if !u.IsAdmin && !authz.VisibleInPicker(false, d.grantsFor(u), path) {
			httpx.Error(w, http.StatusForbidden, "folder_denied")
			return
		}
		var folders []syno.Folder
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			f, e := c.ListFolder(r.Context(), sid, path)
			folders = f
			return e
		}); err != nil {
			writeNASError(w, err)
			return
		}
		folders = d.filterFolders(u, folders)
		if folders == nil {
			folders = []syno.Folder{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

// handleCreateFolderStateful creates a subfolder, gated by the SAME per-user
// create ACL as adding a task to that path — a user may only create folders
// where they're allowed to download (spec 1006).
func handleCreateFolderStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
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
		// The new folder as a create destination (no leading slash) must clear the
		// same gate as downloading there.
		rel := strings.Trim(body.Path, "/")
		if rel == "" {
			// Refuse creating new top-level shares — that's a NAS admin action.
			httpx.Error(w, http.StatusForbidden, "folder_denied")
			return
		}
		rel = rel + "/" + body.Name
		if _, ok := authz.Normalize(rel); !ok || !d.destinationAllowed(u, rel) {
			httpx.Error(w, http.StatusForbidden, "folder_denied")
			return
		}
		var folder syno.Folder
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			f, e := c.CreateFolder(r.Context(), sid, body.Path, body.Name)
			folder = f
			return e
		}); err != nil {
			writeNASError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folder": folder})
	})
}

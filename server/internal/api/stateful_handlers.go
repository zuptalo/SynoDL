package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// These mirror the legacy task/fs handlers but authenticate the caller as a
// SynoDL user and reach the NAS through the shared connection manager (which
// owns the DSM session) instead of a client-supplied sid. Folder-scope
// enforcement is added in Increment 3.

func handleListTasksStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
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
		if tasks == nil {
			tasks = []syno.Task{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"tasks": tasks, "stats": stats})
	})
}

func handleCreateTaskStateful(d Deps) http.Handler {
	maxBytes := int64(d.Cfg.MaxTorrentMB) << 20
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
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
			if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
				return c.CreateTaskFile(r.Context(), sid, header.Filename, file, opts)
			}); err != nil {
				writeNASError(w, err)
				return
			}
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
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			return c.CreateTaskURIs(r.Context(), sid, uris, opts)
		}); err != nil {
			writeNASError(w, err)
			return
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
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var folders []syno.Folder
		if err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
			f, e := c.ListShares(r.Context(), sid)
			folders = f
			return e
		}); err != nil {
			writeNASError(w, err)
			return
		}
		if folders == nil {
			folders = []syno.Folder{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

func handleListFolderStateful(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		path := r.URL.Query().Get("path")
		if path == "" {
			httpx.Error(w, http.StatusBadRequest, "path required")
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
		if folders == nil {
			folders = []syno.Folder{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"folders": folders})
	})
}

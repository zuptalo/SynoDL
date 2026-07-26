package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"synodl/server/internal/httpx"
	"synodl/server/internal/syno"
)

// handleListTasks returns the task list AND the global speed pair in one
// response. The PWA polls this on an interval; bundling both halves the
// request rate from every installed client, which matters on a NAS. The two
// NAS calls run concurrently; a stats failure degrades to zeros rather than
// failing the whole poll (the list is the part the user is looking at).
func handleListTasks(d Deps) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		var (
			wg    sync.WaitGroup
			tasks []syno.Task
			stats syno.Stats
			tErr  error
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			tasks, tErr = d.Syno.ListTasks(r.Context(), sid)
		}()
		go func() {
			defer wg.Done()
			stats, _ = d.Syno.Stats(r.Context(), sid)
		}()
		wg.Wait()
		if tErr != nil {
			writeSynoError(w, tErr)
			return
		}
		if tasks == nil {
			tasks = []syno.Task{}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"tasks": tasks,
			"stats": stats,
		})
	})
}

// createTaskJSON is the JSON body for URL-based task creation.
type createTaskJSON struct {
	URIs          []string `json:"uris"`
	Destination   string   `json:"destination"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	UnzipPassword string   `json:"unzipPassword"`
}

// handleCreateTask accepts either a JSON body (URL/magnet list) or a
// multipart form (a .torrent/NZB upload in the "torrent" field). The upload is
// capped by MAX_TORRENT_MB via MaxBytesReader so an oversized body is refused
// while streaming, not after buffering.
func handleCreateTask(d Deps) http.Handler {
	maxBytes := int64(d.Cfg.MaxTorrentMB) << 20
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
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
			opts := syno.CreateOpts{
				Destination:   r.FormValue("destination"),
				UnzipPassword: r.FormValue("unzipPassword"),
			}
			if err := d.Syno.CreateTaskFile(r.Context(), sid, header.Filename, file, opts); err != nil {
				writeSynoError(w, err)
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
			Destination:   req.Destination,
			Username:      req.Username,
			Password:      req.Password,
			UnzipPassword: req.UnzipPassword,
		}
		if err := d.Syno.CreateTaskURIs(r.Context(), sid, uris, opts); err != nil {
			writeSynoError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

// handleTaskAction covers pause/resume/delete — same body shape, same
// response, different verb forwarded to the NAS.
func handleTaskAction(d Deps, action string) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
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
		var err error
		switch action {
		case "pause":
			err = d.Syno.PauseTasks(r.Context(), sid, req.IDs)
		case "resume":
			err = d.Syno.ResumeTasks(r.Context(), sid, req.IDs)
		case "delete":
			err = d.Syno.DeleteTasks(r.Context(), sid, req.IDs)
		}
		if err != nil {
			writeSynoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

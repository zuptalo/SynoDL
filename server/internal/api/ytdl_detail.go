package api

import (
	"errors"
	"net/http"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// Opening one download (spec 0013, US4).
//
// Everything known about it in one answer, so the client never has to assemble a
// picture from the list plus a second call. The fields that may genuinely be
// unknown stay pointers all the way to the wire, so absent reaches the client as
// absent (FR-033).

// handleYtdlDetail returns one download.
//
// Answers 404 for a download the caller may not see — never 403. A 403 confirms
// the thing exists, which is the disclosure FR-008 is about, so "not yours" and
// "no such id" are the same answer here as they are for dismissal.
func handleYtdlDetail(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		rec, ok := d.ytdlRecordFor(w, r, u)
		if !ok {
			return
		}
		live, _ := d.ytdlLiveJobs(r.Context())
		state, reason := liveStateOf(rec, live)
		d.captureTerminal(rec, state, reason)
		httpx.JSON(w, http.StatusOK, d.ytdlViewOf(rec, state, reason, u))
	})
}

// ytdlGroupView is a group plus a page of its items (FR-019a).
type ytdlGroupView struct {
	Group      ytdlDownloadView   `json:"group"`
	Items      []ytdlDownloadView `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

// handleYtdlItems returns a group's items.
//
// This is where an expanded playlist or channel's contents live: the Tasks list
// shows the GROUP as one row (FR-019b), because with no ceiling on expansion a
// flat list would let one channel push everything else off the screen. Opening
// the row is what reveals the items.
func handleYtdlItems(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		rec, ok := d.ytdlRecordFor(w, r, u)
		if !ok {
			return
		}
		if rec.Kind != store.YtdlKindGroup {
			// Not a group, so it has no items. 404 rather than an empty list:
			// asking for the contents of something that cannot have contents is
			// a wrong address, not an empty answer.
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}

		items, next, err := d.Store.ListYtdlItems(rec.RequestID, r.URL.Query().Get("cursor"), 100)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}

		live, _ := d.ytdlLiveJobs(r.Context())
		views := make([]ytdlDownloadView, 0, len(items))
		for _, it := range items {
			state, reason := liveStateOf(it, live)
			d.captureTerminal(it, state, reason)
			views = append(views, d.ytdlViewOf(it, state, reason, u))
		}

		groupState, groupReason := liveStateOf(rec, live)
		httpx.JSON(w, http.StatusOK, ytdlGroupView{
			Group:      d.ytdlViewOf(rec, groupState, groupReason, u),
			Items:      views,
			NextCursor: next,
		})
	})
}

// ytdlRecordFor loads the addressed download if the caller may see it, and
// writes the response itself when they may not.
//
// Shared by every single-download endpoint so the ownership rule cannot drift
// apart action by action (FR-008).
func (d Deps) ytdlRecordFor(w http.ResponseWriter, r *http.Request, u *store.User) (store.YtdlDownload, bool) {
	requestID := r.PathValue("requestId")
	if requestID == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid request")
		return store.YtdlDownload{}, false
	}
	rec, err := d.Store.GetYtdlDownload(requestID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not found")
		return store.YtdlDownload{}, false
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return store.YtdlDownload{}, false
	}
	if !ytdlVisibleTo(u, rec.UserID) {
		httpx.Error(w, http.StatusNotFound, "not found")
		return store.YtdlDownload{}, false
	}
	return rec, true
}

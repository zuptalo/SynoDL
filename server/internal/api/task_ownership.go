package api

import (
	"strings"
	"time"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// Per-user task ownership (spec 1013). Download Station has no concept of which
// SynoDL user created a task, so SynoDL attributes tasks itself: a claim is
// recorded at create time and the push watcher stamps the owner on the task when
// it first appears. This file projects that ownership onto the Tasks list —
// labelling each row "added by <user>" for admins, and hiding other people's
// tasks from anyone whose effective scope is "own".

// claimBridgeWindowSecs matches the watcher's attribution window: a just-created
// task is treated as its creator's own (via a pending claim) for this long,
// covering the gap before the watcher attributes it durably.
const claimBridgeWindowSecs = 30 * 60

// taskView is a task plus its (admin-only) attribution and, for downloads sent
// from Discover, the catalog metadata (movie/series, IMDb rating, year). AddedBy
// is omitted for non-admins so a regular user never learns who else uses the
// instance; the media fields are shown to everyone.
type taskView struct {
	syno.Task
	AddedBy   string  `json:"addedBy,omitempty"`
	MediaType string  `json:"mediaType,omitempty"` // movie / series / anime
	IMDbScore float64 `json:"imdbScore,omitempty"`
	Year      string  `json:"year,omitempty"`
}

// asViews wraps tasks with no attribution — used by the legacy stateless stream,
// which has no SynoDL users to attribute to.
func asViews(tasks []syno.Task) []taskView {
	out := make([]taskView, len(tasks))
	for i, t := range tasks {
		out[i] = taskView{Task: t}
	}
	return out
}

// decorateTasks filters and labels the task list for one user:
//   - "own" scope (every non-admin, and admins who opted down): keep only tasks
//     the user created — attributed to them, or matching one of their still-
//     pending claims (a download they just added, not yet attributed).
//   - "any" scope (an admin's default): keep everything.
//   - admins additionally get each task's "added by <user>".
//
// A store error degrades to the raw list (no attribution) rather than failing the
// poll the user is looking at.
func (d Deps) decorateTasks(u *store.User, tasks []syno.Task) []taskView {
	scope, err := d.Store.EffectiveNotificationScope(u.ID, u.IsAdmin)
	if err != nil {
		return asViews(tasks)
	}
	owners, err := d.Store.TaskOwners()
	if err != nil {
		return asViews(tasks)
	}
	var pending map[string]bool
	if scope != "any" {
		pending, _ = d.Store.PendingClaimNames(u.ID, time.Now().Unix()-claimBridgeWindowSecs)
	}
	// Catalog metadata for Discover-sent downloads, keyed by destination folder.
	media, _ := d.Store.SourceDownloads()

	out := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		owner, attributed := owners[t.ID]
		// A task sent from Discover is attributed by its destination FOLDER, which
		// is reliable — the name-based claim can't match it because DSM names the
		// task after the file, not the folder. This is the primary owner source for
		// source downloads.
		md, hasMedia := media[strings.Trim(strings.TrimSpace(t.Destination), "/")]
		if hasMedia && md.OwnerID != 0 {
			owner = store.TaskOwner{UserID: md.OwnerID, Username: md.OwnerName}
			attributed = true
		}
		if scope != "any" {
			mine := attributed && owner.UserID == u.ID
			if !mine && !pending[t.Name] {
				continue // someone else's task — hidden in "own" scope
			}
		}
		v := taskView{Task: t}
		if u.IsAdmin && attributed {
			v.AddedBy = owner.Username
		}
		// Label it movie/series + rating/year from the same folder match.
		if hasMedia {
			v.MediaType = md.MediaType
			v.IMDbScore = md.IMDbScore
			v.Year = md.Year
		}
		out = append(out, v)
	}
	return out
}

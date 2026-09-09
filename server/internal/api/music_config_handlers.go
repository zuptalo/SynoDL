package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// Where music lives on the NAS (spec 1040).
//
// Film and TV parents are inherited from a download source, because a source is
// what puts films there. Music has no source: tracks arrive from YouTube, via a
// worker that mounts a Kubernetes volume the server never sees. So these two are
// an operator decision with nowhere else to come from, and they live on the
// single-row operator config beside the download size cap.
//
// The paths are what make the upload options exist at all. Empty means the
// option is not offered, rather than offered and then failing at the last step.

type musicLibraryView struct {
	Music      string `json:"music"`
	MusicVideo string `json:"musicVideo"`
	// CanManage tells the client whether to render the fields as editable. The
	// server enforces it regardless; this only decides what is worth showing.
	CanManage bool `json:"canManage"`
}

// handleGetMusicLibraries answers for any signed-in user.
//
// Not admin-only, deliberately: the upload sheet has to know whether to offer
// Music at all, and a library's folder name is not a secret — it is visible in
// the destination of every download that lands in it.
func handleGetMusicLibraries(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		m, err := d.Store.GetMusicParents()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, musicLibraryView{
			Music: m.Music, MusicVideo: m.MusicVideo, CanManage: u.IsAdmin,
		})
	})
}

// handleSetMusicLibraries stores them (admin only).
//
// A path is normalised to the shape everything else in the app uses — no leading
// or trailing slash — so the two ends cannot disagree about whether a stored
// path is absolute. Beyond that it is stored as typed: this names a folder that
// already exists on the NAS, and second-guessing an operator's own layout would
// only make a correct path unusable.
func handleSetMusicLibraries(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		var body struct {
			Music      string `json:"music"`
			MusicVideo string `json:"musicVideo"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "bad request")
			return
		}
		music := normalizeLibraryPath(body.Music)
		video := normalizeLibraryPath(body.MusicVideo)
		// Refused rather than repaired, like every other path this app accepts: a
		// value that cannot be a folder path is a mistake worth reporting, not
		// something to quietly turn into a different folder.
		if !validLibraryPath(music) || !validLibraryPath(video) {
			httpx.Error(w, http.StatusBadRequest, "that is not a folder path")
			return
		}
		if err := d.Store.SetMusicParents(store.MusicParents{Music: music, MusicVideo: video}); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, musicLibraryView{Music: music, MusicVideo: video, CanManage: true})
	})
}

func normalizeLibraryPath(p string) string {
	return strings.Trim(strings.TrimSpace(p), "/")
}

// validLibraryPath allows an empty value — that is how a library is turned off —
// and otherwise requires plain segments with no parent references.
func validLibraryPath(p string) bool {
	if p == "" {
		return true
	}
	if len(p) > 512 {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `\`) {
			return false
		}
		for _, r := range seg {
			if r < 0x20 || r == 0x7f {
				return false
			}
		}
	}
	return true
}

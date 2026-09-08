package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// JobRunner is the slice of the orchestrator this feature needs — five calls,
// no more. Handlers and the reconciler depend on this rather than on *k8s.Client
// so tests pass a fake, exactly as every other handler here depends on the small
// syno.Client interface.
//
// The two pod calls are what make a progress bar possible at all: progress, and
// whether a lyrics file was written, exist only in the worker's own output
// (spec 0013). They are used by the reconciler, never by a request handler —
// reading a worker's output must not scale with the number of people looking at
// the page (FR-013f).
type JobRunner interface {
	CreateJob(ctx context.Context, j *k8s.Job) (*k8s.Job, error)
	ListJobs(ctx context.Context, selector string) ([]k8s.Job, error)
	DeleteJob(ctx context.Context, name string) error
	ListPods(ctx context.Context, selector string) ([]k8s.Pod, error)
	PodLog(ctx context.Context, name string, opts k8s.PodLogOptions) ([]byte, error)
}

// ytdlDownloadView is the wire shape of one download.
//
// Spec 0012 deliberately carried no progress here, on the grounds that the
// worker had no channel back to us. It does (spec 0013), so Progress exists —
// but only while a download is actually running. It is a pointer so that
// "nothing is known" is an ABSENT field rather than a confident 0%, which is
// what FR-013 is about: an unreadable log must not look like a stalled download.
type ytdlDownloadView struct {
	RequestID string `json:"requestId"`
	ParentID  string `json:"parentId,omitempty"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	// What it is, when the source would say (spec 1034). Omitted rather than
	// empty, so the client renders its fallback on absence instead of on "".
	Title    string `json:"title,omitempty"`
	Uploader string `json:"uploader,omitempty"`
	Artwork  string `json:"artwork,omitempty"`
	Mode     string `json:"mode"`
	Scope    string `json:"scope"`
	State    string `json:"state"`
	// GroupName is the playlist or channel an expanded item came from (FR-019c).
	GroupName   string   `json:"groupName,omitempty"`
	Progress    *float64 `json:"progress,omitempty"`
	HasLyrics   *bool    `json:"hasLyrics,omitempty"`
	LyricsLang  string   `json:"lyricsLang,omitempty"`
	Attempts    int      `json:"attempts,omitempty"`
	SubmittedBy string   `json:"submittedBy,omitempty"`
	SubmittedAt int64    `json:"submittedAt,omitempty"`
	// FinishedAt is a POINTER for the same reason Progress is: a download that
	// has not finished has no final timestamp, and 0 would render as 1970.
	FinishedAt *int64 `json:"finishedAt,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// Counts is a group's aggregate — how its items are getting on (FR-019).
	Counts *ytdlCountsView `json:"counts,omitempty"`
}

type ytdlCountsView struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Remaining int `json:"remaining"`
}

type ytdlListView struct {
	Downloads []ytdlDownloadView `json:"downloads"`
	// NextCursor is absent on the last page. History is unbounded (FR-006), so
	// the list is paged rather than returned whole (FR-006a).
	NextCursor string `json:"nextCursor,omitempty"`
	// Degraded means the live half could not be read. Stored records are still
	// returned: a partial answer beats an error page, and this is how an
	// infrastructure problem stays distinguishable from a failed download
	// (FR-032b).
	Degraded bool `json:"degraded"`
}

// ytdlLibraries maps each mode to its operator-configured destination. A
// request carries a MODE, never a path — which is what makes "a music request
// cannot write the video library" structural rather than merely tested.
func (d Deps) ytdlLibraries() map[ytdl.Mode]ytdl.Library {
	libs := map[ytdl.Mode]ytdl.Library{}
	if d.Cfg.YtdlMusicClaim != "" {
		libs[ytdl.ModeMusic] = ytdl.Library{ClaimName: d.Cfg.YtdlMusicClaim, UID: d.Cfg.YtdlUID, GID: d.Cfg.YtdlGID}
	}
	if d.Cfg.YtdlMusicVideoClaim != "" {
		libs[ytdl.ModeMusicVideo] = ytdl.Library{ClaimName: d.Cfg.YtdlMusicVideoClaim, UID: d.Cfg.YtdlUID, GID: d.Cfg.YtdlGID}
	}
	return libs
}

// ytdlAvailable reports whether this deployment can run workers at all.
//
// Kept separate from every other failure mode on purpose: "this deployment does
// not do that" is a 503 with an explanation, not a 500. A Compose or bare
// container install is unaffected by the feature existing.
func (d Deps) ytdlAvailable() bool { return d.Jobs != nil && d.Cfg.YtdlConfigured() }

// ytdlVisibleTo reports whether u may see this download.
//
// The rule is the one NAS tasks already follow: your own, unless you are an
// admin (FR-007, FR-009a). Stated as a single function rather than inline in
// three handlers, so "every action on a single download" (FR-008) cannot drift
// apart action by action.
//
// A download with NO owner — the account was deleted, so the record survives
// unattributed (FR-006d) — is visible to admins only. There is nobody left for
// it to belong to.
func ytdlVisibleTo(u *store.User, ownerID *int64) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	return ownerID != nil && *ownerID == u.ID
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Time-based fallback: uniqueness matters, unpredictability does not —
		// the id is a correlation handle, never a capability.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b[:])
}

// handleYtdlSubmit accepts a link and starts a worker for it.
func handleYtdlSubmit(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var body struct {
			URL  string `json:"url"`
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid request")
			return
		}

		// Validate BEFORE anything else happens. Nothing is created, nothing is
		// stored, and nothing is logged for a link we will not accept.
		mode := ytdl.Mode(body.Mode)
		if !mode.Valid() {
			httpx.Error(w, http.StatusBadRequest, "choose either music or music-video")
			return
		}
		target, err := ytdl.Classify(body.URL)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "that link is not a supported YouTube address")
			return
		}

		if !d.ytdlAvailable() {
			httpx.Error(w, http.StatusServiceUnavailable, "downloading from YouTube is not set up on this server")
			return
		}
		libs := d.ytdlLibraries()
		if _, ok := libs[mode]; !ok {
			httpx.Error(w, http.StatusServiceUnavailable, "that library is not set up on this server")
			return
		}

		// FR-023. Two workers writing one folder concurrently is the failure this
		// prevents; the check is per (link, mode) because audio and video live in
		// different libraries and do not collide.
		live, listErr := d.Jobs.ListJobs(r.Context(), ytdl.Selector)
		if listErr == nil {
			for _, j := range live {
				if j.Metadata.Labels[ytdl.LabelMode] != string(mode) {
					continue
				}
				if j.Metadata.Annotations[ytdl.AnnSourceURL] != target.URL {
					continue
				}
				if !ytdl.StateOf(j).Terminal() {
					httpx.JSON(w, http.StatusConflict, map[string]string{
						"error":     "that link is already downloading",
						"requestId": j.Metadata.Labels[ytdl.LabelRequestID],
					})
					return
				}
			}
		}

		// Ask what this is, briefly. A failure here costs a nicer row and nothing
		// else — it must never be the reason a download does not start (FR-003).
		desc := d.Describer.Describe(r.Context(), target)

		requestID := newRequestID()
		job, err := ytdl.BuildJob(ytdl.JobConfig{
			Namespace:          d.Cfg.YtdlNamespace,
			Image:              d.Cfg.YtdlImage,
			RequestID:          requestID,
			UserID:             strconv.FormatInt(u.ID, 10),
			UserName:           u.Username,
			Desc:               desc,
			Mode:               mode,
			Target:             target,
			Libraries:          libs,
			DeadlineSeconds:    d.Cfg.YtdlDeadlineSeconds,
			TTLSeconds:         d.Cfg.YtdlTTLSeconds,
			MinDurationSeconds: d.Cfg.YtdlMinDurationSeconds,
		})
		if err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "downloading from YouTube is not set up on this server")
			return
		}

		// The record is written BEFORE the job exists (FR-001). If the two ever
		// disagree, the version that leaves a download visible-but-unstarted is
		// far better than the one that leaves a worker running with nothing to
		// show it — the first is recoverable by the user, the second is not
		// recoverable at all.
		if err := d.Store.CreateYtdlDownload(store.YtdlDownload{
			RequestID: requestID,
			Kind:      store.YtdlKindSingle,
			UserID:    &u.ID,
			SourceURL: target.URL,
			VideoID:   target.VideoID(),
			Mode:      string(mode),
			Scope:     string(target.Scope),
			State:     string(ytdl.StateScheduled),
			Title:     desc.Title,
			Uploader:  desc.Uploader,
			Artwork:   desc.Artwork,
			Origin:    store.YtdlOriginDirect,
		}); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "could not record the download")
			return
		}

		// Concurrency is still the cluster's to bound at this point; the durable
		// queue arrives with the admission work and takes this over.
		if _, err := d.Jobs.CreateJob(r.Context(), job); err != nil {
			// The record would otherwise describe a download that never
			// started, with nothing to move it out of that state.
			_, _ = d.Store.DeleteYtdlDownload(requestID, u.ID, false)
			httpx.Error(w, http.StatusBadGateway, "could not start the download")
			return
		}

		httpx.JSON(w, http.StatusAccepted, ytdlSubmitView{
			RequestID: requestID,
			Scope:     string(target.Scope),
			Mode:      string(mode),
			State:     string(ytdl.StateScheduled),
		})
	})
}

type ytdlSubmitView struct {
	RequestID string `json:"requestId"`
	Scope     string `json:"scope"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
}

// handleYtdlList returns a page of the caller's downloads.
//
// Reads the durable record and dresses each row in whatever the orchestrator
// says about it right now. That order matters: the record is what exists, and
// the live job is a fact about it — not the other way round. A download whose
// job was swept an hour ago is therefore an ordinary row rather than a gap.
func handleYtdlList(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		limit := 50
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
			limit = n
		}

		records, next, err := d.Store.ListYtdlDownloads(u.ID, u.IsAdmin, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}

		live, degraded := d.ytdlLiveJobs(r.Context())

		views := make([]ytdlDownloadView, 0, len(records))
		for _, rec := range records {
			state, reason := liveStateOf(rec, live)
			// Recorded on observation, so a download that finishes while nobody
			// is watching is still remembered the moment somebody looks. The
			// write is idempotent and only fires on the transition.
			d.captureTerminal(rec, state, reason)
			views = append(views, d.ytdlViewOf(rec, state, reason, u))
		}

		httpx.JSON(w, http.StatusOK, ytdlListView{Downloads: views, NextCursor: next, Degraded: degraded})
	})
}

// ytdlViewOf projects one durable record, wearing whatever state was derived
// for it, onto the wire.
//
// The record supplies everything about WHAT the download is; the derived state
// supplies where it is now. Fields that may genuinely be unknown are pointers,
// so absent stays absent rather than becoming a confident zero (FR-033).
func (d Deps) ytdlViewOf(rec store.YtdlDownload, state, reason string, u *store.User) ytdlDownloadView {
	v := ytdlDownloadView{
		RequestID:   rec.RequestID,
		ParentID:    rec.ParentID,
		Kind:        string(rec.Kind),
		URL:         rec.SourceURL,
		Title:       rec.Title,
		Uploader:    rec.Uploader,
		Artwork:     rec.Artwork,
		Mode:        rec.Mode,
		Scope:       rec.Scope,
		State:       state,
		GroupName:   rec.GroupName,
		HasLyrics:   rec.HasLyrics,
		LyricsLang:  rec.LyricsLang,
		Attempts:    rec.Attempts,
		SubmittedAt: rec.CreatedAt,
		FinishedAt:  rec.FinishedAt,
		Reason:      reason,
	}
	// A group's own row reports how its items are getting on rather than a state
	// of its own (FR-019).
	if rec.Kind == store.YtdlKindGroup {
		if c, err := d.Store.YtdlCounts(rec.RequestID); err == nil {
			v.Counts = &ytdlCountsView{
				Total: c.Total, Completed: c.Completed, Failed: c.Failed, Remaining: c.Remaining,
			}
		}
	}
	// Who sent it is shown to admins only, matching the existing rule for NAS
	// tasks.
	if u != nil && u.IsAdmin {
		v.SubmittedBy = d.ytdlSubmitterName(rec)
	}
	return v
}

// ytdlSubmitterName resolves the display name for a record's owner.
//
// An unattributed record — the account was deleted (FR-006d) — has no name to
// show, and says so rather than inventing one.
func (d Deps) ytdlSubmitterName(rec store.YtdlDownload) string {
	if rec.UserID == nil {
		return ""
	}
	u, err := d.Store.GetUserByID(*rec.UserID)
	if err != nil {
		return ""
	}
	return u.Username
}

func ytdlUserID(j k8s.Job) *int64 {
	raw := j.Metadata.Annotations[ytdl.AnnSubmittedBy]
	if raw == "" {
		return nil
	}
	var id int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return nil
		}
		id = id*10 + int64(c-'0')
	}
	return &id
}

// handleYtdlDismiss removes the RECORD of a download.
//
// It never touches the media library: dismissing forgets that a download
// happened, it does not delete what the download produced.
//
// Spec 0012 refused while a download was still running. Spec 0013 does not
// (FR-005c): with a queue, and with groups that can hold hundreds of items,
// refusing would leave no way to call off work already started. The record goes
// at once; a worker already running is left to FINISH rather than being killed
// mid-write, and the reconciler removes what it leaves behind.
func handleYtdlDismiss(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		requestID := r.PathValue("requestId")
		if requestID == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid request")
			return
		}

		// Ownership lives in the DELETE's WHERE clause, so "not yours" and "not
		// there" are the same answer and cannot be told apart (FR-008).
		removed, err := d.Store.DeleteYtdlDownload(requestID, u.ID, u.IsAdmin)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if !removed {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}

		// A finished job can be swept now. A running one is deliberately left
		// alone: deleting it would strand its worker mid-write, and the files it
		// has already produced are the user's, not ours to abandon halfway.
		if d.Jobs != nil {
			if jobs, err := d.Jobs.ListJobs(r.Context(), ytdl.Selector); err == nil {
				for _, j := range jobs {
					if j.Metadata.Labels[ytdl.LabelRequestID] != requestID {
						continue
					}
					if ytdl.StateOf(j).Terminal() {
						_ = d.Jobs.DeleteJob(r.Context(), j.Metadata.Name)
					}
				}
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

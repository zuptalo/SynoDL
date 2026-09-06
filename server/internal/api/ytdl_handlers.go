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

// JobRunner is the slice of the orchestrator these handlers need — three calls,
// no more. Handlers depend on this rather than on *k8s.Client so tests pass a
// fake, exactly as every other handler here depends on the small syno.Client
// interface.
type JobRunner interface {
	CreateJob(ctx context.Context, j *k8s.Job) (*k8s.Job, error)
	ListJobs(ctx context.Context, selector string) ([]k8s.Job, error)
	DeleteJob(ctx context.Context, name string) error
}

// ytdlDownloadView is the wire shape of one download.
//
// Note what is NOT here: no size, no downloaded, no speed, no percentage, no
// estimate. Their ABSENCE is the requirement (FR-017) — a zero-valued field
// would be an invitation to start populating it later.
type ytdlDownloadView struct {
	RequestID   string `json:"requestId"`
	URL         string `json:"url"`
	Mode        string `json:"mode"`
	Scope       string `json:"scope"`
	State       string `json:"state"`
	SubmittedBy string `json:"submittedBy,omitempty"`
	SubmittedAt int64  `json:"submittedAt,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type ytdlListView struct {
	Downloads []ytdlDownloadView `json:"downloads"`
	// Degraded means the live half could not be read. Stored failures are still
	// returned: a partial answer beats an error page.
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

		requestID := newRequestID()
		job, err := ytdl.BuildJob(ytdl.JobConfig{
			Namespace:          d.Cfg.YtdlNamespace,
			Image:              d.Cfg.YtdlImage,
			RequestID:          requestID,
			UserID:             strconv.FormatInt(u.ID, 10),
			UserName:           u.Username,
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

		// There is deliberately no server-side queue. Concurrency is bounded by
		// the cluster: a job beyond available resources stays Pending, which is
		// exactly "waiting rather than failing", and it needs no state here.
		if _, err := d.Jobs.CreateJob(r.Context(), job); err != nil {
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

// handleYtdlList returns live downloads merged with unresolved failures.
//
// The live half is ONE label-selector LIST, whatever the history size — which
// is what makes a server restart a non-event (FR-019): there is no bookkeeping
// to reload, because there is no bookkeeping.
func handleYtdlList(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		var (
			views    []ytdlDownloadView
			seen     = map[string]bool{}
			degraded bool
		)

		if d.Jobs != nil {
			jobs, err := d.Jobs.ListJobs(r.Context(), ytdl.Selector)
			if err != nil {
				degraded = true
			}
			for _, j := range jobs {
				v := d.ytdlViewOf(j, u)
				seen[v.RequestID] = true
				if v.State == string(ytdl.StateFailed) {
					// Record on observation. Polling means we see the same
					// failure repeatedly, so this MUST be idempotent — it is,
					// by request id.
					_ = d.Store.RecordYtdlFailure(store.YtdlFailure{
						RequestID: v.RequestID,
						UserID:    ytdlUserID(j),
						SourceURL: v.URL,
						Mode:      v.Mode,
						Scope:     v.Scope,
						Reason:    v.Reason,
					})
				}
				views = append(views, v)
			}
		}

		// Failures whose job has already been swept. A success needs no such
		// row: the files it wrote are its record.
		stored, err := d.Store.ListYtdlFailures()
		if err == nil {
			for _, f := range stored {
				if seen[f.RequestID] {
					continue
				}
				views = append(views, ytdlDownloadView{
					RequestID:   f.RequestID,
					URL:         f.SourceURL,
					Mode:        f.Mode,
					Scope:       f.Scope,
					State:       string(ytdl.StateFailed),
					SubmittedAt: f.FailedAt,
					Reason:      f.Reason,
				})
			}
		}

		if views == nil {
			views = []ytdlDownloadView{}
		}
		httpx.JSON(w, http.StatusOK, ytdlListView{Downloads: views, Degraded: degraded})
	})
}

func (d Deps) ytdlViewOf(j k8s.Job, u *store.User) ytdlDownloadView {
	state := ytdl.StateOf(j)
	v := ytdlDownloadView{
		RequestID: j.Metadata.Labels[ytdl.LabelRequestID],
		URL:       j.Metadata.Annotations[ytdl.AnnSourceURL],
		Mode:      j.Metadata.Labels[ytdl.LabelMode],
		Scope:     j.Metadata.Labels[ytdl.LabelScope],
		State:     string(state),
	}
	if state == ytdl.StateFailed {
		v.Reason = ytdl.FailureReason(j)
	}
	// Who sent it is shown to admins only, matching the existing rule for NAS
	// tasks.
	if u != nil && u.IsAdmin {
		v.SubmittedBy = j.Metadata.Annotations[ytdl.AnnSubmittedByName]
	}
	return v
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

// handleYtdlDismiss removes the RECORD of a finished download.
//
// It never touches the media library: dismissing forgets that a download
// happened, it does not delete what the download produced.
func handleYtdlDismiss(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		requestID := r.PathValue("requestId")
		if requestID == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid request")
			return
		}

		var found bool
		if d.Jobs != nil {
			jobs, err := d.Jobs.ListJobs(r.Context(), ytdl.Selector)
			if err == nil {
				for _, j := range jobs {
					if j.Metadata.Labels[ytdl.LabelRequestID] != requestID {
						continue
					}
					found = true
					if !ytdl.StateOf(j).Terminal() {
						// Deleting a running job would strand its worker
						// mid-write. Cancellation is deliberately out of scope.
						httpx.Error(w, http.StatusConflict, "that download is still running")
						return
					}
					if err := d.Jobs.DeleteJob(r.Context(), j.Metadata.Name); err != nil {
						httpx.Error(w, http.StatusBadGateway, "could not dismiss the download")
						return
					}
				}
			}
		}

		removed, err := d.Store.DeleteYtdlFailure(requestID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if !found && !removed {
			httpx.Error(w, http.StatusNotFound, "not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

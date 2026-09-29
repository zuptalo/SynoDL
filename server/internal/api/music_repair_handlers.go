package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/k8s"
	"synodl/server/internal/musicrepair"
	"synodl/server/internal/store"
)

// Running the music library repair from Settings (spec 1053).
//
// The rules this file holds to, each one a constitution rule made structural:
//
//   - Admin only, on every route, enforced here (requireAdmin), not by the client.
//   - The server never mounts the library. It learns about a run ONLY from the Job
//     it launched and from the fixed events the worker prints, and what it reads is
//     parsed into a fixed shape (musicrepair.ParseLog) — never forwarded, never logged.
//   - Only one run at a time: the database holds the slot (a partial unique index),
//     the cluster is checked for a run started from the command line, and the
//     tool's own lock on the share is the last line.
//   - No new permission: Jobs create/list, pods get/list, pods/log get, as the
//     download workers already use.

// repairState is the feature's in-memory state. Losing it costs a lookup.
type repairState struct {
	mu       sync.Mutex
	image    string
	imageAt  time.Time // when we last tried and FAILED; successes are kept for good
	progress map[string]cachedProgress
}

type cachedProgress struct {
	p  *musicrepair.Progress
	at time.Time
}

func newRepairState() *repairState { return &repairState{progress: map[string]cachedProgress{}} }

const (
	repairHistoryShown = 50
	// How long a failed attempt to learn our own image is remembered, so a broken
	// Role does not become a request to the API server on every poll.
	imageRetryAfter   = 30 * time.Second
	repairProgressTTL = 2 * time.Second
	ownPodContainer   = "synodl"
)

// podGetter is the one extra call this feature needs of the orchestrator. It is a
// separate, optional interface — a type assertion — so the download workers' fakes
// and JobRunner stay exactly as they were.
type podGetter interface {
	GetPod(ctx context.Context, name string) (*k8s.Pod, error)
}

func (d Deps) nowUnix() int64 {
	if d.now != nil {
		return d.now().Unix()
	}
	return time.Now().Unix()
}

// repairUnavailable returns "" when the feature can run, else why it cannot, and
// the image the worker's init container will use.
func (d Deps) repairUnavailable(ctx context.Context) (image, reason string) {
	if !d.Stateful || d.Store == nil || d.Jobs == nil || d.Cfg.YtdlImage == "" || d.Cfg.YtdlMusicClaim == "" {
		return "", "not_configured"
	}
	if img := d.selfImage(ctx); img != "" {
		return img, ""
	}
	return "", "no_image"
}

// selfImage is the image THIS server is running. The worker copies the repair
// tool out of it, so the tool's version is the server's version by construction.
func (d Deps) selfImage(ctx context.Context) string {
	if d.Cfg.MusicRepairImage != "" {
		return d.Cfg.MusicRepairImage
	}
	rs := d.repair
	if rs == nil {
		return ""
	}
	rs.mu.Lock()
	if rs.image != "" {
		defer rs.mu.Unlock()
		return rs.image
	}
	if !rs.imageAt.IsZero() && time.Since(rs.imageAt) < imageRetryAfter {
		defer rs.mu.Unlock()
		return ""
	}
	rs.mu.Unlock()

	img := ""
	if g, ok := d.Jobs.(podGetter); ok {
		if name := os.Getenv("HOSTNAME"); name != "" {
			if p, err := g.GetPod(ctx, name); err == nil {
				img = imageOfPod(p)
			}
		}
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if img != "" {
		rs.image = img
	} else {
		rs.imageAt = time.Now()
	}
	return img
}

// imageOfPod picks the image the server container is actually running: the digest
// reference the runtime reports where it gives one, else the configured image.
func imageOfPod(p *k8s.Pod) string {
	if p == nil {
		return ""
	}
	digest := func(id string) string {
		id = strings.TrimPrefix(id, "docker-pullable://")
		if strings.Contains(id, "@sha256:") {
			return id
		}
		return "" // a bare "sha256:…" is not something a Job can pull
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Name == ownPodContainer {
			if d := digest(cs.ImageID); d != "" {
				return d
			}
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if d := digest(cs.ImageID); d != "" {
			return d
		}
	}
	for _, c := range p.Spec.Containers {
		if c.Name == ownPodContainer && c.Image != "" {
			return c.Image
		}
	}
	if len(p.Spec.Containers) > 0 {
		return p.Spec.Containers[0].Image
	}
	return ""
}

// ---- reading the runs ---------------------------------------------------------

func toRun(r store.MusicRepair) musicrepair.Run {
	run := musicrepair.Run{ID: r.ID, Kind: r.Kind, PlanID: r.PlanID, State: r.State, StartedAt: r.StartedAt}
	if r.FinishedAt != nil {
		run.FinishedAt = *r.FinishedAt
	}
	if r.Summary != "" {
		// Our own re-serialised, bounded shape — but decoded defensively anyway.
		var s musicrepair.Summary
		if json.Unmarshal([]byte(r.Summary), &s) == nil {
			run.Summary = &s
			if run.PlanID == "" {
				run.PlanID = s.PlanID
			}
		}
	}
	return run
}

func (d Deps) repairRuns() ([]store.MusicRepair, []musicrepair.Run, error) {
	rows, err := d.Store.ListMusicRepairs(repairHistoryShown)
	if err != nil {
		return nil, nil, err
	}
	runs := make([]musicrepair.Run, len(rows))
	for i, r := range rows {
		runs[i] = toRun(r)
	}
	return rows, runs, nil
}

// ---- the views ------------------------------------------------------------------

type repairRunView struct {
	ID         string                `json:"id"`
	Kind       string                `json:"kind"`
	State      string                `json:"state"`
	StartedAt  int64                 `json:"startedAt"`
	FinishedAt *int64                `json:"finishedAt,omitempty"`
	StartedBy  string                `json:"startedBy"`
	PlanID     string                `json:"planId,omitempty"`
	Progress   *musicrepair.Progress `json:"progress,omitempty"`
	Headline   string                `json:"headline,omitempty"`
}

type repairPlanView struct {
	ID          string               `json:"id"`
	CheckedAt   int64                `json:"checkedAt"`
	ExpiresAt   int64                `json:"expiresAt"`
	Status      string               `json:"status"`
	CanApply    bool                 `json:"canApply"`
	CanContinue bool                 `json:"canContinue"`
	PlanFile    string               `json:"planFile,omitempty"`
	Summary     *musicrepair.Summary `json:"summary,omitempty"`
}

type repairUndoView struct {
	PlanID    string `json:"planId"`
	AppliedAt int64  `json:"appliedAt"`
	CanUndo   bool   `json:"canUndo"`
}

type repairSnapshot struct {
	Available bool            `json:"available"`
	Reason    string          `json:"reason"`
	Current   *repairRunView  `json:"current"`
	Plan      *repairPlanView `json:"plan"`
	Undo      *repairUndoView `json:"undo"`
	History   []repairRunView `json:"history"`
}

func startedBy(r store.MusicRepair) string {
	if r.UserName != "" {
		return r.UserName
	}
	return "(removed user)"
}

func runView(r store.MusicRepair) repairRunView {
	return repairRunView{ID: r.ID, Kind: r.Kind, State: r.State, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
		StartedBy: startedBy(r), PlanID: toRun(r).PlanID, Headline: headline(r)}
}

// headline is one plain sentence about how a run went. It is built from the fixed
// summary and fixed words only — never from anything the worker wrote as text.
func headline(r store.MusicRepair) string {
	switch r.State {
	case musicrepair.StateRunning:
		return ""
	case musicrepair.StateUnfinished:
		return "Did not finish"
	}
	run := toRun(r)
	s := run.Summary
	if s == nil {
		return "Finished, no summary available"
	}
	if r.State == musicrepair.StateRefused {
		switch s.Reason {
		case "locked":
			return "Refused: another repair holds the lock"
		case "no_plan":
			return "Refused: that plan was not found"
		case "no_space":
			return "Refused: not enough free space"
		}
		return "Refused"
	}
	switch {
	case s.Check != nil:
		return fmt.Sprintf("%d duplicates, %d moves, %d playlists", s.Check.Duplicates, s.Check.Moves, s.Check.Playlists)
	case s.Apply != nil:
		return fmt.Sprintf("%d done, %d skipped, %d failed", s.Apply.Done, s.Apply.Skipped, s.Apply.Failed)
	case s.Undo != nil:
		return fmt.Sprintf("%d restored, %d skipped", s.Undo.Restored, s.Undo.Skipped)
	}
	return "Finished"
}

// ---- GET the snapshot -------------------------------------------------------------

func handleRepairSnapshot(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		ctx := r.Context()
		if _, reason := d.repairUnavailable(ctx); reason != "" {
			httpx.JSON(w, http.StatusOK, repairSnapshot{Available: false, Reason: reason, History: []repairRunView{}})
			return
		}
		rows, runs, err := d.repairRuns()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		// Bring a running record up to date with the cluster now, so the screen is
		// current within one poll rather than one reconciler tick.
		if cur := musicrepair.Active(runs); cur != nil {
			for _, row := range rows {
				if row.ID == cur.ID {
					if d.syncMusicRepair(ctx, row) != row.State {
						rows, runs, err = d.repairRuns()
						if err != nil {
							httpx.Error(w, http.StatusInternalServerError, "server")
							return
						}
					}
				}
			}
		}
		now := d.nowUnix()
		snap := repairSnapshot{Available: true, History: make([]repairRunView, 0, len(rows))}
		for _, row := range rows {
			v := runView(row)
			if row.State == musicrepair.StateRunning {
				v.Progress = d.repairProgress(ctx, row.ID)
				cur := v
				snap.Current = &cur
			}
			snap.History = append(snap.History, v)
		}
		if p := musicrepair.DerivePlan(runs, now); p != nil {
			pv := &repairPlanView{ID: p.ID, CheckedAt: p.CheckedAt, ExpiresAt: p.ExpiresAt, Status: p.Status,
				CanApply: p.CanApply, CanContinue: p.CanContinue, Summary: p.Summary}
			if p.Summary != nil {
				pv.PlanFile = p.Summary.PlanFile
			}
			snap.Plan = pv
		}
		if u := musicrepair.DeriveUndo(runs); u != nil {
			snap.Undo = &repairUndoView{PlanID: u.PlanID, AppliedAt: u.AppliedAt, CanUndo: u.CanUndo}
		}
		httpx.JSON(w, http.StatusOK, snap)
	})
}

// repairProgress is the latest reading of a running run, from the worker's own
// output. Cached for a moment so several watchers do not multiply the reads, and
// lost on restart without consequence.
func (d Deps) repairProgress(ctx context.Context, id string) *musicrepair.Progress {
	rs := d.repair
	if rs == nil {
		return nil
	}
	rs.mu.Lock()
	if c, ok := rs.progress[id]; ok && time.Since(c.at) < repairProgressTTL {
		rs.mu.Unlock()
		return c.p
	}
	rs.mu.Unlock()

	var p *musicrepair.Progress
	if raw := d.repairLog(ctx, id); raw != nil {
		p, _ = musicrepair.ParseLog(raw)
	}
	rs.mu.Lock()
	rs.progress[id] = cachedProgress{p: p, at: time.Now()}
	rs.mu.Unlock()
	return p
}

// repairLog reads the bounded tail of a run's worker output, or nil.
//
// The bytes returned are handed only to ParseLog. They are not logged and not
// kept; a read that fails is simply "nothing to report yet".
func (d Deps) repairLog(ctx context.Context, id string) []byte {
	pods, err := d.Jobs.ListPods(ctx, musicrepair.LabelRepairID+"="+id)
	if err != nil {
		return nil
	}
	for _, p := range pods {
		if p.Metadata.Labels[musicrepair.LabelRepairID] != id {
			continue
		}
		raw, err := d.Jobs.PodLog(ctx, p.Metadata.Name, k8s.PodLogOptions{Container: "repair", TailLines: 200})
		if err != nil {
			return nil
		}
		return raw
	}
	return nil
}

// ---- POST: start a run ------------------------------------------------------------

func handleRepairCheck(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		d.startRepair(w, r, u, "check", "")
	})
}

type repairPlanRequest struct {
	PlanID      string `json:"planId"`
	SnapshotAck bool   `json:"snapshotAck"`
}

func decodePlanRequest(r *http.Request) (repairPlanRequest, bool) {
	var b repairPlanRequest
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<10))
	if err := dec.Decode(&b); err != nil || !musicrepair.PlanIDPattern.MatchString(b.PlanID) {
		return b, false
	}
	return b, true
}

func handleRepairApply(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		b, ok := decodePlanRequest(r)
		if !ok {
			repairError(w, http.StatusBadRequest, "bad_request", "That is not a plan id.")
			return
		}
		// Refused HERE, not merely hidden in the screen: the acknowledgement is the
		// operator saying a snapshot exists, and a request without it is refused
		// whatever the client did (spec 1053, clarified).
		if !b.SnapshotAck {
			repairError(w, http.StatusBadRequest, "snapshot_required",
				"Confirm that you have taken a snapshot of the music share first.")
			return
		}
		d.startRepair(w, r, u, "apply", b.PlanID)
	})
}

func handleRepairUndo(d Deps) http.Handler {
	return d.requireAdmin(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		b, ok := decodePlanRequest(r)
		if !ok {
			repairError(w, http.StatusBadRequest, "bad_request", "That is not a plan id.")
			return
		}
		d.startRepair(w, r, u, "undo", b.PlanID)
	})
}

// repairError writes the error envelope. The message is fixed text written here;
// nothing from the orchestrator or a worker is ever put in it.
func repairError(w http.ResponseWriter, status int, code, message string, extra ...any) {
	body := map[string]any{"error": code, "message": message}
	for i := 0; i+1 < len(extra); i += 2 {
		body[fmt.Sprint(extra[i])] = extra[i+1]
	}
	httpx.JSON(w, status, body)
}

func (d Deps) repairBusy(w http.ResponseWriter, who string, at int64, source string) {
	repairError(w, http.StatusConflict, "busy", "A repair is already running.", "startedBy", who, "startedAt", at, "source", source)
}

func newRepairID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (d Deps) startRepair(w http.ResponseWriter, r *http.Request, u *store.User, kind, planID string) {
	ctx := r.Context()
	image, reason := d.repairUnavailable(ctx)
	if reason != "" {
		repairError(w, http.StatusServiceUnavailable, "unavailable", "The music library repair is not available on this installation.")
		return
	}

	// Guards on what the history says.
	rows, runs, err := d.repairRuns()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}
	now := d.nowUnix()
	var gerr error
	switch kind {
	case "apply":
		gerr = musicrepair.CheckApply(runs, planID, now)
	case "undo":
		gerr = musicrepair.CheckUndo(runs, planID)
	default:
		if musicrepair.Active(runs) != nil {
			gerr = musicrepair.ErrBusy
		}
	}
	if gerr != nil {
		d.refuseRepair(w, gerr, rows)
		return
	}

	// A repair started from the command line is invisible to the database, so ask
	// the cluster. The tool's own lock on the share is the last line of defence.
	jobs, err := d.Jobs.ListJobs(ctx, musicrepair.AnySelector)
	if err != nil {
		repairError(w, http.StatusBadGateway, "start_failed", "The cluster could not be reached, so nothing was started.")
		return
	}
	for _, j := range jobs {
		if musicrepair.JobPhase(&j) != musicrepair.PhaseRunning {
			continue
		}
		if j.Metadata.Labels[musicrepair.LabelKind] == "music-repair" && j.Metadata.Labels[musicrepair.LabelManagedBy] == "synodl" {
			d.repairBusy(w, j.Metadata.Annotations[musicrepair.AnnStartedByName], 0, "settings")
		} else {
			d.repairBusy(w, "", 0, "command_line")
		}
		return
	}

	id, err := newRepairID()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}
	uid := u.ID
	if err := d.Store.StartMusicRepair(store.MusicRepair{
		ID: id, Kind: kind, PlanID: planID, UserID: &uid, UserName: u.Username, StartedAt: now,
	}); err != nil {
		if errors.Is(err, store.ErrRepairBusy) {
			who, at := "", int64(0)
			if cur, _ := d.Store.RunningMusicRepair(); cur != nil {
				who, at = startedBy(*cur), cur.StartedAt
			}
			d.repairBusy(w, who, at, "settings")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}

	job, err := musicrepair.BuildJob(musicrepair.JobConfig{
		Namespace: d.namespace(), RunID: id, Kind: kind, PlanID: planID,
		WorkerImage: d.Cfg.YtdlImage, SelfImage: image, ClaimName: d.Cfg.YtdlMusicClaim,
		UID: d.Cfg.YtdlUID, GID: d.Cfg.YtdlGID,
		StartedBy: fmt.Sprint(u.ID), StartedByName: u.Username,
	})
	if err == nil {
		_, err = d.Jobs.CreateJob(ctx, job)
	}
	if err != nil && !k8s.IsConflict(err) {
		// Release the slot: a run that never started must not hold it. The error
		// text is not sent to the client and not logged (it can echo the manifest).
		_, _ = d.Store.DiscardMusicRepair(id)
		slog.Warn("music repair: could not start", "kind", kind)
		repairError(w, http.StatusBadGateway, "start_failed", "The repair could not be started. Nothing was changed.")
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"id": id})
}

// namespace is where workers are created: the configured one, else the pod's own
// (the k8s client fills an empty namespace in).
func (d Deps) namespace() string { return d.Cfg.YtdlNamespace }

func (d Deps) refuseRepair(w http.ResponseWriter, err error, rows []store.MusicRepair) {
	switch {
	case errors.Is(err, musicrepair.ErrBusy):
		who, at := "", int64(0)
		for _, r := range rows {
			if r.State == musicrepair.StateRunning {
				who, at = startedBy(r), r.StartedAt
				break
			}
		}
		d.repairBusy(w, who, at, "settings")
	case errors.Is(err, musicrepair.ErrNoPlan):
		repairError(w, http.StatusConflict, "no_plan", "This server has no such plan. Run a check first.")
	case errors.Is(err, musicrepair.ErrExpired):
		repairError(w, http.StatusConflict, "plan_expired", "That check is more than 24 hours old. Run it again.")
	case errors.Is(err, musicrepair.ErrAlreadyApplied):
		repairError(w, http.StatusConflict, "already_applied", "That plan was already applied.")
	case errors.Is(err, musicrepair.ErrNotUndoable):
		repairError(w, http.StatusConflict, "not_undoable", "There is nothing to undo for that plan.")
	default:
		httpx.Error(w, http.StatusInternalServerError, "server")
	}
}

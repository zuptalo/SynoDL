package api

import (
	"context"
	"errors"
	"strconv"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// The one thing in this feature that runs on a clock (spec 0013).
//
// Everything periodic happens here, in one loop, and that is a deliberate
// choice rather than an accident of where the code landed. Three separate
// tickers — one to admit from the queue, one to read progress, one to capture
// terminal facts — would each LIST the same jobs on their own schedule and race
// each other over the answer. One loop lists once and hands the same snapshot to
// each responsibility in turn.
//
// It also makes FR-013f structural rather than a rule somebody has to remember:
// a worker's output is read HERE, driven by the fact that a download is running.
// No request handler can reach for it, so the number of people watching the page
// cannot change how often the orchestrator is asked.
//
// Nothing in this loop is allowed to be fatal. The orchestrator being briefly
// unreachable is ordinary; a reconciler that exited on it would silently stop
// admitting from the queue for the life of the process.

// YtdlReconcileInterval is how often a cycle runs.
//
// Faster than the library scan's ten minutes, because a progress bar that
// updates every few seconds is the point, and slower than the task stream's one
// second, because a download that takes minutes does not need per-second
// attention. The client polls every five seconds and reads whatever the last
// cycle held.
const YtdlReconcileInterval = 3 * time.Second

// podLogTailLines bounds the read at the source as well as at the reader
// (FR-013e). Progress is in the most recent lines; the start of a long run has
// nothing left to say.
const podLogTailLines = 200

// RunYtdlReconcile drives the cycle until ctx is cancelled.
//
// One cycle runs immediately, so a restart resumes the queue without waiting out
// the first interval — which is what makes FR-023 hold in practice rather than
// only in principle.
func (d Deps) RunYtdlReconcile(ctx context.Context, interval time.Duration) {
	d.reconcileYtdlOnce(ctx)
	if d.ytdlOnTick != nil {
		d.ytdlOnTick()
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.reconcileYtdlOnce(ctx)
			if d.ytdlOnTick != nil {
				d.ytdlOnTick()
			}
		}
	}
}

// reconcileYtdlOnce is a single cycle.
//
// The order matters. Reading output and capturing terminal facts come BEFORE
// admitting new work, so a download that has just finished frees its slot in the
// same cycle rather than the next one — and so the facts that live only in a
// worker's output are captured before anything else could sweep it (FR-013g).
func (d Deps) reconcileYtdlOnce(ctx context.Context) {
	if d.Jobs == nil {
		return
	}

	jobs, err := d.Jobs.ListJobs(ctx, ytdl.Selector)
	if err != nil {
		// Not fatal, and not logged per-cycle: at this interval an outage would
		// fill the log with the same line. The list simply reports as degraded
		// to whoever asks next.
		return
	}

	live := make(map[string]k8s.Job, len(jobs))
	for _, j := range jobs {
		if id := j.Metadata.Labels[ytdl.LabelRequestID]; id != "" {
			live[id] = j
		}
	}

	// Order matters. Reading output and capturing outcomes come BEFORE
	// admission, so a download that has just finished frees its slot in the same
	// cycle rather than the next one.
	d.readWorkerOutput(ctx, jobs)
	d.captureFinished(live)
	d.resolveVanished(live)
	d.resolveExpansions(ctx, jobs)
	d.refreshGroups(ctx)
	d.sweepDismissed(ctx, jobs)
	d.admitQueued(ctx)

	// LAST, deliberately: everything above may have changed what a download
	// shows, so announcing before them would announce the previous cycle's
	// picture. This is also the moment FR-012 is about — the server has just
	// finished learning, and the update leaves now rather than waiting for
	// somebody to ask (spec 1038).
	d.watchYtdl(live)
}

// resolveVanished fails downloads whose worker the orchestrator has lost.
//
// A record saying "scheduled" or "downloading" with no job behind it means the
// job went away without ever reporting an outcome — the cluster dropped it, or
// something deleted it. Without this the download sits at "starting" forever,
// which FR-013d forbids: every non-final state must be able to reach a final one
// unattended.
//
// It resolves to FAILED, never completed. That is spec 0012's FR-018 instinct
// unchanged: where the evidence of success is missing, resolve against
// ourselves rather than claim a download worked.
func (d Deps) resolveVanished(live map[string]k8s.Job) {
	if d.Store == nil || d.ytdlMissing == nil {
		return
	}
	running, err := d.Store.ListYtdlRunning()
	if err != nil {
		return
	}
	for _, rec := range running {
		if _, ok := live[rec.RequestID]; ok {
			d.ytdlMissing.Present(rec.RequestID)
			continue
		}
		// Two consecutive cycles, several seconds apart, before acting: one
		// absent listing can just be a create that has not propagated.
		if !d.ytdlMissing.Saw(rec.RequestID) {
			continue
		}
		now := time.Now().Unix()
		_ = d.Store.SetYtdlState(rec.RequestID, string(ytdl.StateFailed),
			"the download did not complete", &now)
		d.ytdlMissing.Present(rec.RequestID)
		if d.ytdlProgress != nil {
			d.ytdlProgress.Forget(rec.RequestID)
		}
	}
}

// refreshGroups recomputes each group's state from its items.
//
// A group has no worker of its own once expansion is done, so its state is a
// function of what its items are doing: still working while any item is not
// final, then completed if every item completed, otherwise failed (FR-019).
func (d Deps) refreshGroups(ctx context.Context) {
	if d.Store == nil {
		return
	}
	groups, err := d.Store.ListYtdlActiveGroups()
	if err != nil {
		return
	}
	for _, g := range groups {
		c, err := d.Store.YtdlCounts(g.RequestID)
		if err != nil || c.Total == 0 {
			continue
		}
		if c.Remaining > 0 {
			continue // still working
		}
		state := string(ytdl.StateCompleted)
		reason := ""
		if c.Failed > 0 {
			// Failure is reported before success, everywhere: a group where
			// anything failed did not entirely work, and saying otherwise is
			// the one outcome 0012 FR-018 forbids.
			state = string(ytdl.StateFailed)
			reason = "some items could not be downloaded"
		}
		now := time.Now().Unix()
		if err := d.Store.SetYtdlState(g.RequestID, state, reason, &now); err != nil {
			continue
		}
		// A group notifies ONCE, here, when every item is final (FR-025c). With
		// no ceiling on expansion, notifying per item would turn one paste into
		// several hundred pushes.
		g.State, g.Reason = state, reason
		d.notifyFinished(ctx, g, state)
	}
}

// admitQueued starts as many waiting downloads as there is room for.
//
// SynoDL decides this, rather than handing everything to the cluster and letting
// it queue (which is what spec 0012 did). A cluster-side queue is invisible: the
// user cannot see it, cannot tell it from a slow download, and — once a channel
// can expand without a ceiling — it means dropping several hundred jobs on the
// orchestrator at once.
//
// There is exactly ONE admitter, because SynoDL runs as a single replica
// (deploy/k8s/10-synodl.yaml). That is what lets this be a plain read-then-write
// with no distributed lock. A multi-replica deployment would need one, and is
// out of scope for spec 0013 — the conditional UPDATE in MarkYtdlAdmitted is
// the only nod to it.
func (d Deps) admitQueued(ctx context.Context) {
	if d.Store == nil || d.Jobs == nil {
		return
	}
	limit := d.Cfg.YtdlMaxParallel
	if limit <= 0 {
		limit = 4
	}

	running, err := d.Store.CountYtdlRunning()
	if err != nil {
		return
	}
	free := limit - running
	if free <= 0 {
		return
	}

	queued, err := d.Store.ListYtdlQueued()
	if err != nil || len(queued) == 0 {
		return
	}

	candidates := make([]ytdl.Candidate, 0, len(queued))
	byID := make(map[string]store.YtdlDownload, len(queued))
	for _, q := range queued {
		var uid int64
		if q.UserID != nil {
			uid = *q.UserID
		}
		candidates = append(candidates, ytdl.Candidate{
			RequestID: q.RequestID,
			UserID:    uid,
			Direct:    q.Origin == store.YtdlOriginDirect,
			Seq:       q.QueueSeq,
		})
		byID[q.RequestID] = q
	}

	for _, c := range ytdl.Admit(candidates, free) {
		rec := byID[c.RequestID]
		// Claim the slot BEFORE creating the job. If the create then fails the
		// download is marked failed with a reason, which is recoverable by
		// retrying; the other order could hand the orchestrator a job that no
		// record believes is running.
		claimed, err := d.Store.MarkYtdlAdmitted(rec.RequestID)
		if err != nil || !claimed {
			continue
		}
		if err := d.startYtdlJob(ctx, rec); err != nil {
			now := time.Now().Unix()
			_ = d.Store.SetYtdlState(rec.RequestID, string(ytdl.StateFailed), "could not be started", &now)
		}
	}
}

// startYtdlJob builds and creates the worker for one admitted download.
func (d Deps) startYtdlJob(ctx context.Context, rec store.YtdlDownload) error {
	libs := d.ytdlLibraries()
	mode := ytdl.Mode(rec.Mode)
	if _, ok := libs[mode]; !ok {
		return errNoLibraryConfigured
	}

	var userID, userName string
	if rec.UserID != nil {
		userID = strconv.FormatInt(*rec.UserID, 10)
		if u, err := d.Store.GetUserByID(*rec.UserID); err == nil {
			userName = u.Username
		}
	}

	job, err := ytdl.BuildJob(ytdl.JobConfig{
		Namespace: d.Cfg.YtdlNamespace,
		Image:     d.Cfg.YtdlImage,
		RequestID: rec.RequestID,
		UserID:    userID,
		UserName:  userName,
		Desc:      ytdl.Description{Title: rec.Title, Uploader: rec.Uploader, Artwork: rec.Artwork},
		Mode:      mode,
		Target:    ytdl.Target{URL: rec.SourceURL, Scope: ytdl.Scope(rec.Scope)},
		Libraries: libs,

		GroupName: rec.GroupName,

		DeadlineSeconds:    d.Cfg.YtdlDeadlineSeconds,
		TTLSeconds:         d.Cfg.YtdlTTLSeconds,
		MinDurationSeconds: d.Cfg.YtdlMinDurationSeconds,
	})
	if err != nil {
		return err
	}
	if _, err := d.Jobs.CreateJob(ctx, job); err != nil {
		if k8s.IsConflict(err) {
			// The job already exists: this download was admitted on an earlier
			// cycle and the record is catching up. Not a failure.
			return nil
		}
		return err
	}
	return nil
}

var errNoLibraryConfigured = errors.New("no media library configured for that mode")

// resolveExpansions moves groups through `resolving`.
//
// A group is submitted as a request nobody can start yet: SynoDL does not know
// what it contains. It gets an enumeration worker, and when that finishes its
// output becomes one download record per entry. That is the whole of expansion
// on this side — the enumeration itself, and reading it, live in internal/ytdl.
func (d Deps) resolveExpansions(ctx context.Context, jobs []k8s.Job) {
	if d.Store == nil || d.Jobs == nil {
		return
	}

	// Index the enumeration workers by request id.
	expandJobs := map[string]k8s.Job{}
	for _, j := range jobs {
		if j.Metadata.Labels[ytdl.LabelJobKind] != ytdl.JobKindExpand {
			continue
		}
		if id := j.Metadata.Labels[ytdl.LabelRequestID]; id != "" {
			expandJobs[id] = j
		}
	}

	groups, err := d.Store.ListYtdlResolving()
	if err != nil {
		return
	}
	for _, g := range groups {
		j, running := expandJobs[g.RequestID]
		if !running {
			// No worker yet: either it has never been started, or it finished
			// and was swept before we read it. Starting one is idempotent — the
			// orchestrator answers 409 for a name that already exists.
			d.startExpansion(ctx, g)
			continue
		}
		switch ytdl.StateOf(j) {
		case ytdl.StateFailed:
			// FR-013d: a group cannot sit in `resolving` forever. If the
			// enumeration failed, so did the request — plainly, and with a
			// reason the user can act on.
			now := time.Now().Unix()
			_ = d.Store.SetYtdlState(g.RequestID, string(ytdl.StateFailed),
				"could not read what that link contains", &now)
			_ = d.Jobs.DeleteJob(ctx, j.Metadata.Name)
		case ytdl.StateCompleted:
			d.expandInto(ctx, g, j)
		}
	}
}

// startExpansion creates the enumeration worker for a group.
func (d Deps) startExpansion(ctx context.Context, g store.YtdlDownload) {
	job, err := ytdl.BuildExpansionJob(ytdl.JobConfig{
		Namespace: d.Cfg.YtdlNamespace,
		Image:     d.Cfg.YtdlImage,
		RequestID: g.RequestID,
		Mode:      ytdl.Mode(g.Mode),
		Target:    ytdl.Target{URL: g.SourceURL, Scope: ytdl.Scope(g.Scope)},
	})
	if err != nil {
		now := time.Now().Unix()
		_ = d.Store.SetYtdlState(g.RequestID, string(ytdl.StateFailed), "could not be started", &now)
		return
	}
	if _, err := d.Jobs.CreateJob(ctx, job); err != nil && !k8s.IsConflict(err) {
		now := time.Now().Unix()
		_ = d.Store.SetYtdlState(g.RequestID, string(ytdl.StateFailed), "could not be started", &now)
	}
}

// expandInto turns a finished enumeration into one queued download per entry.
func (d Deps) expandInto(ctx context.Context, g store.YtdlDownload, j k8s.Job) {
	raw, err := d.podLogFor(ctx, j)
	if err != nil {
		// The output is gone before we read it. Failing the group is the honest
		// outcome: an empty expansion would look like "this channel has nothing
		// in it", which is a different and wrong statement.
		now := time.Now().Unix()
		_ = d.Store.SetYtdlState(g.RequestID, string(ytdl.StateFailed),
			"could not read what that link contains", &now)
		return
	}

	entries := ytdl.ParseEntries(raw)

	// A CHANNEL publishes no metadata document, so nothing was learned about it
	// at submission and its group has no name yet. Every entry knows who
	// published it, and that is the channel's real name — unlike the tab name a
	// playlist_title would have given (spec 0012 research, §5).
	groupName := g.GroupName
	if groupName == "" {
		for _, e := range entries {
			if n := ytdl.SanitizeName(e.Uploader); n != "" {
				groupName = n
				break
			}
		}
	}

	items := make([]store.YtdlDownload, 0, len(entries))
	for _, e := range entries {
		// Already held: a completed record for this item in this mode means
		// re-running the channel should not fetch it again (FR-020). Checked
		// HERE, before anything is queued, so a re-run creates no rows that
		// would immediately finish having done nothing.
		held, err := d.Store.YtdlAlreadyHeld(e.ID, g.Mode)
		if err == nil && held {
			continue
		}
		items = append(items, store.YtdlDownload{
			RequestID: newRequestID(),
			ParentID:  g.RequestID,
			Kind:      store.YtdlKindItem,
			UserID:    g.UserID,
			SourceURL: e.URL,
			VideoID:   e.ID,
			Mode:      g.Mode,
			Scope:     string(ytdl.ScopeSingle),
			State:     string(ytdl.StateQueued),
			Title:     e.Title,
			// Artwork is derived from the item's id, so a channel of any size
			// costs no extra requests to illustrate (FR-002).
			Artwork:   ytdl.ThumbnailFor(e.ID),
			Uploader:  e.Uploader,
			GroupName: groupName,
			Origin:    store.YtdlOriginExpanded,
		})
	}

	// One transaction, whatever the size. A channel with no ceiling can be
	// thousands of rows, and a half-inserted group would leave a request that is
	// neither resolving nor complete.
	if err := d.Store.CreateYtdlItems(g.RequestID, items); err != nil {
		return // try again next cycle; the group stays resolving
	}
	// The group takes the name too, so its own row reads as the channel rather
	// than as a bare link.
	if groupName != "" && g.GroupName == "" {
		_ = d.Store.SetYtdlGroupName(g.RequestID, groupName)
	}
	_ = d.Jobs.DeleteJob(ctx, j.Metadata.Name)
}

// podLogFor reads the output of a job's pod.
func (d Deps) podLogFor(ctx context.Context, j k8s.Job) ([]byte, error) {
	pods, err := d.Jobs.ListPods(ctx, ytdl.Selector)
	if err != nil {
		return nil, err
	}
	want := j.Metadata.Labels[ytdl.LabelRequestID]
	for _, p := range pods {
		if p.Metadata.Labels[ytdl.LabelRequestID] != want {
			continue
		}
		if p.Metadata.Labels[ytdl.LabelJobKind] != ytdl.JobKindExpand {
			continue
		}
		return d.Jobs.PodLog(ctx, p.Metadata.Name, k8s.PodLogOptions{
			Container: "downloader",
			// An enumeration's whole output matters, not just its tail — the
			// first entries are as much part of the answer as the last.
			TailLines: 0,
		})
	}
	return nil, errExpansionOutputGone
}

var errExpansionOutputGone = errors.New("expansion output is no longer available")

// captureFinished writes the outcome of anything that has just become final.
//
// This is what makes a download's history survive the orchestrator sweeping its
// job — and it runs on the clock rather than only when somebody looks, so a
// download that finishes overnight is recorded overnight (FR-002, FR-003).
//
// A write that fails is deliberately swallowed. FR-006b: a download that has
// already saved its files must never be REPORTED as failed because the server
// could not write a row about it. The files exist either way, and the next cycle
// tries again.
func (d Deps) captureFinished(live map[string]k8s.Job) {
	if d.Store == nil {
		return
	}
	for id, j := range live {
		// A group's own state is derived from its items, never from a job — the
		// enumeration worker is not the group.
		if j.Metadata.Labels[ytdl.LabelJobKind] == ytdl.JobKindExpand {
			continue
		}
		state := ytdl.StateOf(j)
		if !state.Terminal() {
			continue
		}
		rec, err := d.Store.GetYtdlDownload(id)
		if err != nil {
			// No record: a job this server did not create, or one whose record
			// the user has already dismissed. Neither is ours to invent a row for.
			continue
		}
		reason := ""
		if state == ytdl.StateFailed {
			reason = ytdl.FailureReason(j)
		}
		d.captureTerminal(rec, string(state), reason)

		// A finished download has no progress. Dropping the entry also stops the
		// cache growing by one key per download for the life of the process.
		if d.ytdlProgress != nil {
			d.ytdlProgress.Forget(id)
		}
	}
}

// sweepDismissed deletes jobs whose record the user has dismissed.
//
// Dismissing removes the record at once, but a RUNNING worker is left to finish
// rather than stranded mid-write (FR-005b). This is what clears up after it: once
// that worker is done, its job has no record left to belong to, so it goes.
func (d Deps) sweepDismissed(ctx context.Context, jobs []k8s.Job) {
	if d.Store == nil {
		return
	}
	for _, j := range jobs {
		id := j.Metadata.Labels[ytdl.LabelRequestID]
		if id == "" || !ytdl.StateOf(j).Terminal() {
			continue
		}
		if _, err := d.Store.GetYtdlDownload(id); err == nil {
			continue // still wanted
		}
		_ = d.Jobs.DeleteJob(ctx, j.Metadata.Name)
	}
}

// readWorkerOutput reads what each RUNNING worker says about itself.
//
// A job that has not started has nothing to say and is not asked — which is both
// the cheap thing to do and the honest one: a pod that does not exist yet cannot
// have produced output, so asking would only produce a 404 to discard.
func (d Deps) readWorkerOutput(ctx context.Context, jobs []k8s.Job) {
	var running []k8s.Job
	for _, j := range jobs {
		// An enumeration worker is not a download: it prints entries, not
		// progress, and reading it as one would attribute a group's own
		// enumeration to a download that does not exist.
		if j.Metadata.Labels[ytdl.LabelJobKind] == ytdl.JobKindExpand {
			continue
		}
		if ytdl.StateOf(j) == ytdl.StateDownloading {
			running = append(running, j)
		}
	}
	if len(running) == 0 {
		return
	}

	pods, err := d.Jobs.ListPods(ctx, ytdl.Selector)
	if err != nil {
		return
	}
	// Index pods by the request id their labels carry, so a job is matched to
	// its pod by the same selector that found both — never by guessing at a
	// name the cluster generates.
	podFor := map[string]k8s.Pod{}
	for _, p := range pods {
		if id := p.Metadata.Labels[ytdl.LabelRequestID]; id != "" {
			podFor[id] = p
		}
	}

	for _, j := range running {
		p, ok := podFor[j.Metadata.Labels[ytdl.LabelRequestID]]
		if !ok || p.Status.Phase != "Running" {
			continue
		}
		raw, err := d.Jobs.PodLog(ctx, p.Metadata.Name, k8s.PodLogOptions{
			Container: "downloader",
			TailLines: podLogTailLines,
		})
		if err != nil {
			// A swept pod is the ordinary case. Losing a reading must never
			// change what a download reports (FR-013) — the row simply shows no
			// percentage until the next successful read, or forever.
			continue
		}

		id := j.Metadata.Labels[ytdl.LabelRequestID]
		reading := ytdl.ScanOutput(raw)

		if reading.HasProgress && d.ytdlProgress != nil {
			if f, known := reading.Latest.Fraction(); known {
				d.ytdlProgress.Observe(id, f)
			}
		}

		// Whether a lyrics file was written is the one fact here that must
		// OUTLIVE the worker, so it goes to the store rather than the cache: the
		// orchestrator sweeps this pod's output and there is nowhere else the
		// fact exists (FR-011, FR-013g).
		if reading.HasLyrics && d.Store != nil {
			rec, err := d.Store.GetYtdlDownload(id)
			if err == nil && (rec.HasLyrics == nil || !*rec.HasLyrics) {
				_ = d.Store.SetYtdlCompanion(id, true, reading.LyricsLang)
			}
		}
	}
}

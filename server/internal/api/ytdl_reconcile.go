package api

import (
	"context"
	"time"

	"synodl/server/internal/k8s"
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

	d.readWorkerOutput(ctx, jobs)
}

// readWorkerOutput reads what each RUNNING worker says about itself.
//
// A job that has not started has nothing to say and is not asked — which is both
// the cheap thing to do and the honest one: a pod that does not exist yet cannot
// have produced output, so asking would only produce a 404 to discard.
func (d Deps) readWorkerOutput(ctx context.Context, jobs []k8s.Job) {
	var running []k8s.Job
	for _, j := range jobs {
		if ytdl.StateOf(j) == ytdl.StateStarted {
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
		if _, err := d.Jobs.PodLog(ctx, p.Metadata.Name, k8s.PodLogOptions{
			Container: "downloader",
			TailLines: podLogTailLines,
		}); err != nil {
			// A swept pod is the ordinary case. Losing a reading must never
			// change what a download reports (FR-013).
			continue
		}
		// Parsing arrives with the progress work; the read is what this phase
		// establishes.
	}
}

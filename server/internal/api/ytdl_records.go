package api

import (
	"context"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// Reconciling the durable record with what the orchestrator says (spec 0013).
//
// This is the Principle III split made concrete. The store holds what was ASKED
// FOR and how it ENDED. The orchestrator holds what is happening RIGHT NOW.
// Neither is derived from the other: a live job's state is read from the job,
// and a finished download's state is read from the record, and the moment
// between them — a download that has just become final — is the only time
// anything is written.

// liveStateOf returns the state to SHOW for a stored download, given the live
// jobs the orchestrator reported.
//
// A record with a live job reports the job's state, because the job is the truth
// while it exists. A record without one reports its own, because the job has
// been swept and the record is all that is left. That ordering is why a restart
// cannot desynchronise anything: there is no third place to disagree.
func liveStateOf(d store.YtdlDownload, live map[string]k8s.Job) (state string, reason string) {
	// A GROUP has no worker of its own: its enumeration is a step in resolving
	// it, and afterwards its state is a function of its items (refreshGroups).
	// Belt and braces alongside the filter in ytdlLiveJobs — this is the rule,
	// and stating it here means no future caller can reintroduce the collision
	// by building a live map some other way.
	if d.Kind == store.YtdlKindGroup {
		return d.State, d.Reason
	}
	j, ok := live[d.RequestID]
	if !ok {
		return d.State, d.Reason
	}
	s := ytdl.StateOf(j)
	if s == ytdl.StateFailed {
		return string(s), ytdl.FailureReason(j)
	}
	return string(s), ""
}

// captureTerminal writes a download's outcome the first time it is seen final.
//
// Idempotent, and deliberately so: state is polled, so the same finished job is
// observed on every cycle until the orchestrator sweeps it. Writing on every
// sighting would keep moving the timestamp, and the first sighting is the one
// that means anything.
func (d Deps) captureTerminal(rec store.YtdlDownload, state, reason string) {
	if rec.State == state {
		return
	}
	if state != string(ytdl.StateCompleted) && state != string(ytdl.StateFailed) {
		// Not final: state that is still moving belongs to the orchestrator, and
		// writing it here would be the mirror Principle III forbids.
		return
	}
	now := time.Now().Unix()
	if err := d.Store.SetYtdlState(rec.RequestID, state, reason, &now); err != nil {
		// FR-006b: a download that saved its files is not failed because a row
		// could not be written. Nothing is announced either — announcing an
		// outcome we could not record would mean announcing it again next cycle.
		return
	}
	rec.State, rec.Reason = state, reason
	d.notifyFinished(context.Background(), rec, state)
}

// ytdlLiveJobs indexes the orchestrator's jobs by request id.
//
// Returns degraded=true when the orchestrator could not be reached. A partial
// answer beats an error page: stored records are still worth showing, and the
// client is told the live half is missing rather than being shown stale state as
// though it were current (FR-032b).
func (d Deps) ytdlLiveJobs(ctx context.Context) (map[string]k8s.Job, bool) {
	if d.Jobs == nil {
		return nil, false
	}
	jobs, err := d.Jobs.ListJobs(ctx, ytdl.Selector)
	if err != nil {
		return nil, true
	}
	out := make(map[string]k8s.Job, len(jobs))
	for _, j := range jobs {
		// An ENUMERATION job is not any download's worker. It carries the
		// GROUP's request id — it has to, that is how the reconciler finds it —
		// so leaving it in this map made `live[group]` resolve to it, and the
		// moment it succeeded the group was reported as completed. The group
		// then left `resolving` and expansion never ran: a playlist went
		// straight to "saved" having downloaded nothing.
		//
		// A group's state comes from its items, never from a job.
		if ytdlNotADownload(j) {
			continue
		}
		if id := j.Metadata.Labels[ytdl.LabelRequestID]; id != "" {
			out[id] = j
		}
	}
	return out, false
}

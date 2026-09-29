package api

import (
	"context"
	"encoding/json"

	"synodl/server/internal/k8s"
	"synodl/server/internal/musicrepair"
	"synodl/server/internal/store"
)

// syncMusicRepair brings ONE running record up to date with what the cluster says,
// and — the important part — captures a finished run's outcome while its worker's
// output is still readable (constitution v2.2.0: facts that exist only in a
// worker's output are captured before the orchestrator sweeps it).
//
// It is called from two places and is safe to call from both, any number of times:
// the existing download reconciler's tick (one loop, so nothing races over the same
// Jobs list) and the snapshot handler (so the screen is current within one poll).
// Finishing a record applies only once.
//
// It returns the record's state after the sync.
func (d Deps) syncMusicRepair(ctx context.Context, rec store.MusicRepair) string {
	if rec.State != musicrepair.StateRunning || d.Jobs == nil || d.Store == nil {
		return rec.State
	}
	jobs, err := d.Jobs.ListJobs(ctx, musicrepair.Selector)
	if err != nil {
		// Not knowing is not the same as the Job being gone: leave the record be.
		return rec.State
	}
	var job *k8s.Job
	for i := range jobs {
		if jobs[i].Metadata.Name == musicrepair.JobName(rec.ID) {
			job = &jobs[i]
			break
		}
	}
	phase := musicrepair.JobPhase(job)

	var sum *musicrepair.Summary
	if job != nil && (phase == musicrepair.PhaseSucceeded || phase == musicrepair.PhaseFailed) {
		if raw := d.repairLog(ctx, rec.ID); raw != nil {
			_, sum = musicrepair.ParseLog(raw)
		}
	}
	now := d.nowUnix()
	next := musicrepair.NextState(toRun(rec), phase, sum, now)
	if next == musicrepair.StateRunning {
		return rec.State
	}
	encoded := ""
	if sum != nil {
		if b, err := json.Marshal(sum); err == nil {
			encoded = string(b)
		}
	}
	if _, err := d.Store.FinishMusicRepair(rec.ID, next, encoded, now); err != nil {
		return rec.State
	}
	return next
}

// reconcileMusicRepairs is the periodic half: it is called from the download
// reconciler's tick.
func (d Deps) reconcileMusicRepairs(ctx context.Context) {
	if d.Store == nil || d.Jobs == nil {
		return
	}
	cur, err := d.Store.RunningMusicRepair()
	if err != nil || cur == nil {
		return
	}
	d.syncMusicRepair(ctx, *cur)
}

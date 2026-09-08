package api

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/ytdl"
)

// The reconciler is the one thing in this feature that runs on a clock. Its
// contract is small and load-bearing: it must keep going when the orchestrator
// is unreachable, and it must read a worker's output because a download is
// running — never because somebody is looking at the page (FR-013f).

func TestReconcile_RunsOnItsTicker(t *testing.T) {
	jobs := &fakeJobs{}
	d := Deps{Jobs: jobs}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ticks int
	var mu sync.Mutex
	d.ytdlOnTick = func() {
		mu.Lock()
		ticks++
		mu.Unlock()
	}

	go d.RunYtdlReconcile(ctx, 5*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := ticks
		mu.Unlock()
		if n >= 3 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("saw %d ticks, want the loop to keep running", n)
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func TestReconcile_SurvivesAListError(t *testing.T) {
	// The orchestrator being briefly unreachable is ordinary. A reconciler that
	// exited on it would stop admitting from the queue for the life of the
	// process, and nothing would say why.
	jobs := &fakeJobs{listErr: errors.New("apiserver unreachable")}
	d := Deps{Jobs: jobs}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var ticks int
	var mu sync.Mutex
	d.ytdlOnTick = func() {
		mu.Lock()
		ticks++
		mu.Unlock()
	}

	go d.RunYtdlReconcile(ctx, 5*time.Millisecond)
	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	n := ticks
	mu.Unlock()
	if n < 2 {
		t.Fatalf("saw %d ticks, want the loop to survive a list error", n)
	}
}

func TestReconcile_StopsOnContextCancel(t *testing.T) {
	jobs := &fakeJobs{}
	d := Deps{Jobs: jobs}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		d.RunYtdlReconcile(ctx, time.Hour)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not stop when its context was cancelled")
	}
}

func TestReconcile_ReadsOutputForRunningDownloadsOnly(t *testing.T) {
	// FR-013f. Output is read because a download is running. A request handler
	// must never reach for it, or the read would scale with the number of
	// viewers rather than with the amount of work.
	jobs := &fakeJobs{}
	d := Deps{Jobs: jobs}

	// A job that is not running yet has nothing to say, and must not be asked.
	jobs.jobs = append(jobs.jobs, ytdlTestJob("pending-one", nil))

	d.reconcileYtdlOnce(context.Background())

	jobs.mu.Lock()
	reqs := append([]string{}, jobs.logReqs...)
	jobs.mu.Unlock()
	if len(reqs) != 0 {
		t.Fatalf("read output for %v, want nothing read for a job that is not running", reqs)
	}
}

// ytdlTestJob builds a job carrying this feature's labels, in a given status.
func ytdlTestJob(requestID string, status *k8s.JobStatus) k8s.Job {
	j := k8s.Job{
		Metadata: k8s.ObjectMeta{
			Name: ytdl.JobName(requestID),
			Labels: map[string]string{
				ytdl.LabelManagedBy: "synodl",
				ytdl.LabelKind:      "ytdl",
				ytdl.LabelRequestID: requestID,
				ytdl.LabelMode:      string(ytdl.ModeMusic),
				ytdl.LabelScope:     string(ytdl.ScopeSingle),
			},
		},
	}
	if status != nil {
		j.Status = *status
	}
	return j
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

func TestReconcile_CapturesTerminalStateDurably(t *testing.T) {
	// The point of the record: a download's outcome survives the orchestrator
	// sweeping the job that produced it (FR-002, FR-003).
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Succeeded: 1})

	d := Deps{Jobs: jobs, Store: st}
	d.reconcileYtdlOnce(context.Background())

	got, err := st.GetYtdlDownload(sub.RequestID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != "completed" {
		t.Fatalf("state = %q, want completed recorded without anyone looking at the list", got.State)
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt not recorded, so the detail view would have nothing to show")
	}

	// And re-running does not move the timestamp: the first sighting is when it
	// finished.
	at := *got.FinishedAt
	d.reconcileYtdlOnce(context.Background())
	again, _ := st.GetYtdlDownload(sub.RequestID)
	if *again.FinishedAt != at {
		t.Fatalf("finishedAt moved from %d to %d on re-observation", at, *again.FinishedAt)
	}
}

func TestReconcile_SweepsAJobWhoseRecordWasDismissed(t *testing.T) {
	// Dismissing removes the record at once and leaves a running worker to
	// finish (FR-005b). Once it has, its job has no record left to belong to.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)

	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	if r := do(t, h, "DELETE", "/v1/ytdl/"+sub.RequestID, "", admin); r.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d, want 204", r.Code)
	}

	d := Deps{Jobs: jobs, Store: st}
	// Still running: the worker must be left alone.
	d.reconcileYtdlOnce(context.Background())
	if len(jobs.deleted) != 0 {
		t.Fatalf("a running worker was swept mid-write: %v", jobs.deleted)
	}

	// Finished: now there is nothing to strand.
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())
	if len(jobs.deleted) != 1 {
		t.Fatalf("deleted = %v, want the finished job swept once its record was gone", jobs.deleted)
	}
}

func TestReconcile_ARecordThatCannotBeWrittenNeverFailsASavedDownload(t *testing.T) {
	// FR-006b. History is unbounded and the volume is not; if the store cannot
	// be written, the files a download already saved still exist, and reporting
	// it as failed would be a lie about the one thing that did work.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Succeeded: 1})

	// Closing the store is the bluntest possible "cannot write".
	_ = st.Close()

	d := Deps{Jobs: jobs, Store: st}
	d.reconcileYtdlOnce(context.Background()) // must not panic, must not mark failed

	got := listYtdl2(t, h, admin)
	for _, dl := range got.Downloads {
		if dl.State == "failed" {
			t.Fatalf("a download that saved was reported as failed because the record could not be written: %+v", dl)
		}
	}
}

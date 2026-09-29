package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/musicrepair"
	"synodl/server/internal/store"
)

const testPlan = "20260929T071341Z-73c844"

const goodResult = `@@synodl {"event":"result","kind":"check","ok":true,"planId":"` + testPlan + `","planFile":".repair/plan-x.md",` +
	`"check":{"tracks":5795,"songs":4017,"duplicates":1774,"moves":3970,"retags":4017,"playlists":88,"matched":1828,` +
	`"noMatch":2186,"notLookedUp":3,"freeBytes":4600000000000,"leftAlone":{"total":1,"byReason":[{"reason":"no video id","count":1}],` +
	`"examples":[{"path":"A/b.mp3","reason":"no video id"}]}}}` + "\n"

// repairFixture is a store with one RUNNING check and a fake cluster carrying its Job.
type repairFixture struct {
	d    Deps
	st   *store.Store
	jobs *fakeJobs
	id   string
	now  *int64
}

func newRepairFixture(t *testing.T, kind string, jobStatus k8s.JobStatus, log string, withJob bool) *repairFixture {
	t.Helper()
	_, st := newRepairRouter(t, &fakeJobs{}, repairCfg())
	jobs := &fakeJobs{}
	id := "a1b2c3d4e5f6"
	now := int64(1_790_000_000)
	if err := st.StartMusicRepair(store.MusicRepair{ID: id, Kind: kind, UserName: "Anna", StartedAt: now - 300}); err != nil {
		t.Fatal(err)
	}
	if withJob {
		job := k8s.Job{Metadata: k8s.ObjectMeta{Name: musicrepair.JobName(id), Labels: map[string]string{
			musicrepair.LabelManagedBy: "synodl", musicrepair.LabelKind: "music-repair", musicrepair.LabelRepairID: id}}, Status: jobStatus}
		jobs.jobs = append(jobs.jobs, job)
		jobs.pods = append(jobs.pods, k8s.Pod{Metadata: k8s.ObjectMeta{Name: "pod-" + id,
			Labels: map[string]string{musicrepair.LabelRepairID: id}}})
		jobs.logs = map[string]string{"pod-" + id: log}
	}
	f := &repairFixture{st: st, jobs: jobs, id: id, now: &now}
	f.d = Deps{Cfg: repairCfg(), Stateful: true, Store: st, Jobs: jobs, repair: newRepairState(),
		now: func() time.Time { return time.Unix(*f.now, 0) }}
	return f
}

func (f *repairFixture) row(t *testing.T) *store.MusicRepair {
	t.Helper()
	r, err := f.st.GetMusicRepair(f.id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return r
}

var complete = k8s.JobStatus{Succeeded: 1, Conditions: []k8s.JobCondition{{Type: "Complete", Status: "True"}}}
var failed = k8s.JobStatus{Failed: 1, Conditions: []k8s.JobCondition{{Type: "Failed", Status: "True"}}}

func TestReconcileRepair_ACompletedJobIsCapturedWithItsSummary(t *testing.T) {
	f := newRepairFixture(t, "check", complete, "[plan] noise\n"+goodResult, true)
	f.d.reconcileMusicRepairs(context.Background())
	r := f.row(t)
	if r.State != "finished" || r.FinishedAt == nil {
		t.Fatalf("row = %+v, want finished", r)
	}
	var s musicrepair.Summary
	if err := json.Unmarshal([]byte(r.Summary), &s); err != nil {
		t.Fatalf("stored summary: %v", err)
	}
	if s.PlanID != testPlan || s.Check == nil || s.Check.Duplicates != 1774 || s.Check.LeftAlone.Total != 1 {
		t.Errorf("summary = %+v", s)
	}
	if strings.Contains(r.Summary, "noise") || strings.Contains(r.Summary, "@@synodl") {
		t.Errorf("stored summary = %s: only the decoded shape may be stored", r.Summary)
	}
}

func TestReconcileRepair_AToolRefusalIsRefusedNotUnfinished(t *testing.T) {
	log := `@@synodl {"event":"result","kind":"check","ok":false,"reason":"locked"}` + "\n"
	f := newRepairFixture(t, "check", failed, log, true) // exit 4 fails the Job
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "refused" {
		t.Fatalf("state = %s, want refused", r.State)
	}
}

func TestReconcileRepair_FailedStepsStillMeansItFinished(t *testing.T) {
	log := `@@synodl {"event":"result","kind":"apply","ok":false,"reason":"failed_steps","apply":{"done":5,"failed":2}}` + "\n"
	f := newRepairFixture(t, "apply", failed, log, true) // exit 3 fails the Job
	f.d.reconcileMusicRepairs(context.Background())
	r := f.row(t)
	if r.State != "finished" || !strings.Contains(r.Summary, `"failed":2`) {
		t.Fatalf("row = %+v, want finished with the failed count", r)
	}
}

func TestReconcileRepair_AFailedJobWithNoReportDidNotFinish(t *testing.T) {
	f := newRepairFixture(t, "check", failed, "[plan] scanning\n", true) // a deadline, an eviction, a crash
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "unfinished" || r.Summary != "" {
		t.Fatalf("row = %+v, want unfinished with no summary", r)
	}
}

func TestReconcileRepair_ARunningJobIsLeftAlone(t *testing.T) {
	f := newRepairFixture(t, "check", k8s.JobStatus{Active: 1}, "", true)
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "running" {
		t.Fatalf("state = %s", r.State)
	}
}

func TestReconcileRepair_AVanishedJobIsUnfinishedOnlyAfterTheGracePeriod(t *testing.T) {
	f := newRepairFixture(t, "check", k8s.JobStatus{}, "", false)
	// The row is 300 s old in the fixture; make it brand new.
	*f.now = 1_790_000_000
	if _, err := f.st.DiscardMusicRepair(f.id); err != nil {
		t.Fatal(err)
	}
	_ = f.st.StartMusicRepair(store.MusicRepair{ID: f.id, Kind: "check", UserName: "Anna", StartedAt: *f.now - 10})
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "running" {
		t.Fatalf("within the grace period: state = %s, want still running (the Job is created moments after the row)", r.State)
	}
	*f.now += musicrepair.StartGrace + 5
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "unfinished" {
		t.Fatalf("after the grace period: state = %s, want unfinished, never finished", r.State)
	}
}

func TestReconcileRepair_AMissingOrMalformedReportOnACompletedJobIsFinishedWithNoSummary(t *testing.T) {
	for name, log := range map[string]string{
		"no events":  "[plan] done\n",
		"malformed":  "@@synodl {\"event\":\"result\",\n",
		"wrong kind": `@@synodl {"event":"result","kind":"nuke","ok":true}` + "\n",
	} {
		f := newRepairFixture(t, "check", complete, log, true)
		f.d.reconcileMusicRepairs(context.Background())
		if r := f.row(t); r.State != "finished" || r.Summary != "" {
			t.Errorf("%s: row = %+v, want finished with no summary", name, r)
		}
	}
}

func TestReconcileRepair_AnOutcomeIsCapturedOnceAndNeverRewritten(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	f.d.reconcileMusicRepairs(context.Background())
	first := f.row(t)
	f.jobs.mu.Lock()
	f.jobs.logs["pod-"+f.id] = `@@synodl {"event":"result","kind":"check","ok":false,"reason":"locked"}` + "\n"
	f.jobs.mu.Unlock()
	f.d.reconcileMusicRepairs(context.Background())
	again := f.row(t)
	if again.Summary != first.Summary || again.State != first.State {
		t.Errorf("a recorded outcome changed on a later tick: %+v → %+v", first, again)
	}
}

func TestReconcileRepair_ARestartPicksTheRunBackUpWithItsTrueState(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	// A brand-new Deps over the same store and the same cluster: a server restart.
	fresh := Deps{Cfg: repairCfg(), Stateful: true, Store: f.st, Jobs: f.jobs, repair: newRepairState(),
		now: f.d.now}
	fresh.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "finished" {
		t.Fatalf("state = %s: a run that ended while the server was down must be found on return (SC-004)", r.State)
	}
}

func TestReconcileRepair_ItNeverTouchesDownloadOrUnrelatedJobs(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	f.jobs.jobs = append(f.jobs.jobs,
		k8s.Job{Metadata: k8s.ObjectMeta{Name: "ytdl-abc", Labels: map[string]string{"synodl.io/kind": "ytdl"}}, Status: failed},
		k8s.Job{Metadata: k8s.ObjectMeta{Name: "someone-elses", Labels: map[string]string{"app": "other"}}, Status: failed})
	f.d.reconcileMusicRepairs(context.Background())
	if len(f.jobs.deleted) != 0 {
		t.Errorf("deleted %v: this feature manages nothing but its own Job, and deletes nothing at all", f.jobs.deleted)
	}
}

func TestReconcileRepair_ANoticeOfAnOutageIsNotAConclusion(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	f.jobs.mu.Lock()
	f.jobs.listErr = &k8s.APIError{Status: 500}
	f.jobs.mu.Unlock()
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "running" {
		t.Fatalf("state = %s: not being able to ask is not the same as the Job being gone", r.State)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestReconcileRepair_WorkerOutputNeverReachesTheServersLog(t *testing.T) {
	var buf syncBuf
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	log := "[plan] SECRET-HUMAN-LINE /Volumes/private\n" + strings.Replace(goodResult, "no video id", "SECRET-REASON", 1) +
		"@@synodl {not json SECRET-BROKEN\n"
	f := newRepairFixture(t, "check", complete, log, true)
	f.d.reconcileMusicRepairs(context.Background())
	// and a start that fails, whose error text can echo a manifest
	h, _ := newRepairRouter(t, &fakeJobs{createErr: &k8s.APIError{Status: 500, Message: "SECRET-MANIFEST-ECHO"}}, repairCfg())
	admin := adminAfterSetup(t, h)
	startCheck(t, h, admin)

	for _, forbidden := range []string{"SECRET-HUMAN-LINE", "SECRET-REASON", "SECRET-BROKEN", "SECRET-MANIFEST-ECHO", "/Volumes/private", "@@synodl"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Errorf("the server log contains %q (FR-022): %s", forbidden, buf.String())
		}
	}
}

func TestReconcileRepair_ItRunsOnTheDownloadReconcilersTick_OneLoop(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	f.d.ytdlOnTick = func() {}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.d.RunYtdlReconcile(ctx, 10*time.Millisecond); close(done) }()
	deadline := time.After(3 * time.Second)
	for {
		if f.row(t).State == "finished" {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("the existing reconciler loop never captured the run: it must call the repair capture on its own tick")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

func TestReconcileRepair_ItStillRunsWhenListingDownloadWorkersFails(t *testing.T) {
	f := newRepairFixture(t, "check", complete, goodResult, true)
	// The download half returns early on a list error; the repair half must not
	// depend on it, and this fake fails ONLY the download selector.
	fj := &selectorFailingJobs{fakeJobs: f.jobs}
	f.d.Jobs = fj
	f.d.reconcileYtdlOnce(context.Background())
	f.d.reconcileMusicRepairs(context.Background())
	if r := f.row(t); r.State != "finished" {
		t.Fatalf("state = %s", r.State)
	}
}

type selectorFailingJobs struct{ *fakeJobs }

func (s *selectorFailingJobs) ListJobs(ctx context.Context, sel string) ([]k8s.Job, error) {
	if strings.Contains(sel, "kind=ytdl") {
		return nil, &k8s.APIError{Status: 500}
	}
	return s.fakeJobs.ListJobs(ctx, sel)
}

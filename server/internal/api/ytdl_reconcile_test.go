package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
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
	ytdlAdmit(t, jobs, st)
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
	ytdlAdmit(t, jobs, st)

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
	ytdlAdmit(t, jobs, st)
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

// ytdlRunning submits a download and drives it to a running worker with a pod
// ready to be read, returning its request id.
func ytdlRunning(t *testing.T, h http.Handler, jobs *fakeJobs, st *store.Store, auth map[string]string, url string) string {
	t.Helper()
	rec := submit(t, h, auth, `{"url":"`+url+`","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	// Submitting only queues now; admission is what creates the worker.
	ytdlAdmit(t, jobs, st)
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	jobs.attachPod(sub.RequestID)
	return sub.RequestID
}

func TestReconcile_ReportsProgressFromWorkerOutput(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Jobs: jobs, Store: st})
	jobs.emitFor(id, ytdl.ProgressSentinel+" status=downloading downloaded=25 total=100")
	d.reconcileYtdlOnce(context.Background())

	got, ok := d.ytdlProgress.Get(id)
	if !ok {
		t.Fatal("no progress held after reading a worker line")
	}
	if got < 0.24 || got > 0.26 {
		t.Fatalf("progress = %v, want about a quarter", got)
	}

	// A later, LOWER reading must not move the bar backwards (FR-012).
	jobs.emitFor(id, ytdl.ProgressSentinel+" status=downloading downloaded=5 total=100")
	d.reconcileYtdlOnce(context.Background())
	after, _ := d.ytdlProgress.Get(id)
	if after < got {
		t.Fatalf("progress went backwards, %v then %v", got, after)
	}
}

func TestReconcile_RecordsLyricsAndItsLanguageDurably(t *testing.T) {
	// FR-011 + FR-013g. This fact exists ONLY in the worker's output, and the
	// orchestrator sweeps that — so it has to be captured while it is readable.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Jobs: jobs, Store: st})
	jobs.emitFor(id, `[info] Writing video subtitles to: /out/Queen/Singles/Bohemian Rhapsody.en-orig.lrc`)
	d.reconcileYtdlOnce(context.Background())

	rec, err := st.GetYtdlDownload(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.HasLyrics == nil || !*rec.HasLyrics {
		t.Fatal("lyrics were written and should have been recorded")
	}
	if rec.LyricsLang != "en" {
		t.Fatalf("lyrics language = %q, want en", rec.LyricsLang)
	}
}

func TestReconcile_AnUnreadableLogLeavesTheStateAloneAndShowsNoProgress(t *testing.T) {
	// FR-013. Losing a reading is not a download failing, and must not look like
	// one — nor like a download frozen at 0%.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	jobs.logErr = errors.New("forbidden: pods/log")
	d := InitCaches(Deps{Jobs: jobs, Store: st})
	d.reconcileYtdlOnce(context.Background())

	if _, ok := d.ytdlProgress.Get(id); ok {
		t.Fatal("progress was invented despite the log being unreadable")
	}
	rec, err := st.GetYtdlDownload(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.State == "failed" {
		t.Fatal("an unreadable log reported the download as failed")
	}
}

func TestReconcile_ForgetsProgressOnceFinished(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Jobs: jobs, Store: st})
	jobs.emitFor(id, ytdl.ProgressSentinel+" status=downloading downloaded=50 total=100")
	d.reconcileYtdlOnce(context.Background())
	if _, ok := d.ytdlProgress.Get(id); !ok {
		t.Fatal("expected progress to be held while running")
	}

	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())
	if _, ok := d.ytdlProgress.Get(id); ok {
		t.Fatal("a finished download still holds progress; the cache would grow a key per download forever")
	}
}

// The queue (spec 0013, US7). Spec 0012 had none: concurrency was "bounded by
// the cluster", which is a real bound but an invisible one — and once a channel
// can expand without a ceiling it means handing the orchestrator hundreds of
// jobs at once.

func ytdlQueue(t *testing.T, h http.Handler, auth map[string]string, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		rec := submit(t, h, auth, `{"url":"https://youtu.be/song`+itoa2(i)+`","mode":"music"}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("submit %d = %d %s", i, rec.Code, rec.Body.String())
		}
		var sub ytdlSubmitResp
		_ = json.Unmarshal(rec.Body.Bytes(), &sub)
		ids = append(ids, sub.RequestID)
	}
	return ids
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestAdmit_NeverRunsMoreThanTheLimit(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	_ = ytdlQueue(t, h, admin, 10)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.admitQueued(context.Background())

	if len(jobs.created) != 4 {
		t.Fatalf("started %d workers, want the configured limit of 4", len(jobs.created))
	}
	// Running again with nothing finished must not start more.
	d.admitQueued(context.Background())
	if len(jobs.created) != 4 {
		t.Fatalf("a second pass started more: %d", len(jobs.created))
	}
}

func TestAdmit_AFinishInEitherStateStartsTheNext(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	ids := ytdlQueue(t, h, admin, 6)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	if len(jobs.created) != 4 {
		t.Fatalf("started %d, want 4", len(jobs.created))
	}

	// One succeeds, one fails: BOTH free a slot, because the limit is about
	// what is running, not about what worked.
	jobs.setStatus(t, ids[0], k8s.JobStatus{Succeeded: 1})
	jobs.setStatus(t, ids[1], k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())

	if len(jobs.created) != 6 {
		t.Fatalf("started %d after two finished, want the next two admitted", len(jobs.created))
	}
}

func TestAdmit_RespectsTheOperatorsLimit(t *testing.T) {
	cfg := ytdlCfg()
	cfg.YtdlMaxParallel = 2
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, cfg)
	admin := adminAfterSetup(t, h)
	_ = ytdlQueue(t, h, admin, 5)

	d := InitCaches(Deps{Cfg: cfg, Store: st, Jobs: jobs})
	d.admitQueued(context.Background())
	if len(jobs.created) != 2 {
		t.Fatalf("started %d, want the operator's limit of 2", len(jobs.created))
	}
}

func TestAdmit_ResumesAfterARestartWithoutDoubleStarting(t *testing.T) {
	// FR-023. The queue is durable precisely so a restart does not strand it —
	// and the thing to get wrong is starting everything twice on the way back.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	_ = ytdlQueue(t, h, admin, 6)

	before := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	before.admitQueued(context.Background())
	if len(jobs.created) != 4 {
		t.Fatalf("started %d, want 4", len(jobs.created))
	}

	// A "restart": brand new Deps, so every in-memory cache is gone. The store
	// and the orchestrator are all that survive — which is the point.
	after := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	after.admitQueued(context.Background())

	if len(jobs.created) != 4 {
		t.Fatalf("after a restart %d workers exist, want the same 4 — the queue resumed rather than restarted", len(jobs.created))
	}
	// And the two still waiting are still waiting, not lost.
	queued, err := st.ListYtdlQueued()
	if err != nil {
		t.Fatalf("list queued: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("%d downloads still queued, want the 2 that never started", len(queued))
	}
}

func TestAdmit_SharesSlotsBetweenUsersEndToEnd(t *testing.T) {
	// SC-005a, through the real store: one user's bulk work must not make
	// another user wait for all of it.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	_ = ytdlQueue(t, h, admin, 10)
	boRec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	var boSub ytdlSubmitResp
	_ = json.Unmarshal(boRec.Body.Bytes(), &boSub)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.admitQueued(context.Background())

	rec, err := st.GetYtdlDownload(boSub.RequestID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.State == "queued" {
		t.Fatal("Bo's single link is still waiting behind ten of the admin's — fair-share did not apply")
	}
}

func TestAdmit_SkipsADeletedAccountsQueuedWork(t *testing.T) {
	// FR-006d. The record survives as history; the work does not, because
	// nobody is waiting for it and it would occupy a slot someone else needs.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)

	users, _ := st.ListUsers()
	for _, u := range users {
		if u.Username == "bo" {
			if err := st.DeleteUser(u.ID); err != nil {
				t.Fatalf("delete user: %v", err)
			}
		}
	}

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.admitQueued(context.Background())

	if len(jobs.created) != 0 {
		t.Fatalf("started %d workers for a deleted account", len(jobs.created))
	}
	// The record is still there, unattributed.
	got, err := st.GetYtdlDownload(sub.RequestID)
	if err != nil {
		t.Fatalf("the record should survive the account: %v", err)
	}
	if got.UserID != nil {
		t.Fatalf("UserID = %d, want it detached", *got.UserID)
	}
}

// Expansion (spec 0013, US6). A playlist or channel stops being one opaque bulk
// job and becomes the items it contains — each with its own progress, its own
// outcome, and its own retry.

// submitGroup submits a channel link and returns its request id.
func submitGroup(t *testing.T, h http.Handler, auth map[string]string, url string) string {
	t.Helper()
	rec := submit(t, h, auth, `{"url":"`+url+`","mode":"music"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit group = %d %s", rec.Code, rec.Body.String())
	}
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	if sub.Kind != "group" || sub.State != "resolving" {
		t.Fatalf("submit = %+v, want a group that is resolving", sub)
	}
	return sub.RequestID
}

// expansionOutput builds enumeration output for the given ids.
func expansionOutput(uploader string, ids ...string) string {
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(ytdl.ExpandSentinel + " id=" + id + " uploader=" + uploader + " title=Track " + id + "\n")
	}
	return b.String()
}

func TestExpand_TurnsAChannelIntoItsItems(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})

	// First cycle starts the enumeration worker; it mounts no library.
	d.reconcileYtdlOnce(context.Background())
	if len(jobs.created) != 1 {
		t.Fatalf("created %d jobs, want the enumeration worker", len(jobs.created))
	}
	if len(jobs.created[0].Spec.Template.Spec.Volumes) != 0 {
		t.Error("the enumeration worker mounted a media library; it writes nothing")
	}

	// It finishes, having listed three items.
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc")

	d.reconcileYtdlOnce(context.Background())

	items, _, err := st.ListYtdlItems(gid, "", 100)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expanded into %d items, want 3", len(items))
	}
	for _, it := range items {
		// Queued, or already admitted — expansion and admission happen in the
		// same cycle on purpose, so an item need not wait a tick to start.
		if it.State != "queued" && it.State != "scheduled" {
			t.Errorf("item %s = %q, want it queued or admitted", it.RequestID, it.State)
		}
		if it.Scope != "single" {
			t.Errorf("item %s scope = %q, want single", it.RequestID, it.Scope)
		}
		// FR-037: the group's name travels with each item, so the album is
		// right even though the item is fetched with no playlist context.
		if it.GroupName != "Lo-fi Beats" {
			t.Errorf("item %s groupName = %q, want the channel's name", it.RequestID, it.GroupName)
		}
	}

	// The group itself now has the channel's name, learned from its contents.
	g, _ := st.GetYtdlDownload(gid)
	if g.GroupName != "Lo-fi Beats" || g.State == "resolving" {
		t.Fatalf("group = %+v, want it named and out of resolving", g)
	}
}

func TestExpand_SkipsWhatIsAlreadyHeld(t *testing.T) {
	// FR-020. Re-running a channel queues only what is new — checked BEFORE
	// anything is queued, so no rows are created that would immediately finish
	// having done nothing.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	owner := adminUserID(t, st)

	// Already have one of them.
	held := store.YtdlDownload{
		RequestID: "old", Kind: store.YtdlKindSingle, UserID: &owner,
		SourceURL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", VideoID: "aaaaaaaaaaa",
		Mode: "music", Scope: "single", State: "completed",
	}
	if err := st.CreateYtdlDownload(held); err != nil {
		t.Fatal(err)
	}

	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())

	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa", "bbbbbbbbbbb")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	if len(items) != 1 {
		t.Fatalf("queued %d items, want only the one not already held", len(items))
	}
	if items[0].VideoID != "bbbbbbbbbbb" {
		t.Fatalf("queued %q, want the new item", items[0].VideoID)
	}
}

func TestExpand_ADismissedRecordMeansTheItemIsNoLongerHeld(t *testing.T) {
	// FR-020a. Dismissing forgets a download; asking for it again is a real
	// request, not a duplicate.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	owner := adminUserID(t, st)

	if err := st.CreateYtdlDownload(store.YtdlDownload{
		RequestID: "old", Kind: store.YtdlKindSingle, UserID: &owner,
		SourceURL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", VideoID: "aaaaaaaaaaa",
		Mode: "music", Scope: "single", State: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	if ok, err := st.DeleteYtdlDownload("old", owner, false); err != nil || !ok {
		t.Fatalf("dismiss: %v %v", ok, err)
	}

	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	if len(items) != 1 {
		t.Fatalf("queued %d items, want the dismissed one fetched again", len(items))
	}
}

func TestExpand_AFailedEnumerationFailsTheGroupPlainly(t *testing.T) {
	// FR-013d: nothing may sit in `resolving` forever. And an empty expansion
	// would say "this channel has nothing in it", which is a different and
	// wrong statement.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	jobs.setStatusByName(t, ytdl.ExpandJobName(gid), k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())

	g, _ := st.GetYtdlDownload(gid)
	if g.State != "failed" {
		t.Fatalf("group = %q, want it failed rather than stuck resolving", g.State)
	}
	if g.Reason == "" {
		t.Error("a failed expansion must say something the user can act on")
	}
	for _, forbidden := range []string{"yt-dlp", "--", "/out"} {
		if strings.Contains(g.Reason, forbidden) {
			t.Errorf("reason %q leaks %q", g.Reason, forbidden)
		}
	}
}

func TestExpand_OneFailedItemLeavesTheRestAlone(t *testing.T) {
	// FR-018. The whole reason for expanding: a single bad entry used to take
	// the entire channel with it.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	// They get admitted, then one fails.
	d.reconcileYtdlOnce(context.Background())
	jobs.setStatus(t, items[0].RequestID, k8s.JobStatus{Failed: 1})
	jobs.setStatus(t, items[1].RequestID, k8s.JobStatus{Succeeded: 1})
	jobs.setStatus(t, items[2].RequestID, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())

	c, err := st.YtdlCounts(gid)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if c.Completed != 2 || c.Failed != 1 {
		t.Fatalf("counts = %+v, want 2 saved and 1 failed", c)
	}
	// The group reports failed BECAUSE something failed — reporting an
	// unsuccessful run as completed is the one outcome 0012 FR-018 forbids.
	g, _ := st.GetYtdlDownload(gid)
	if g.State != "failed" {
		t.Fatalf("group = %q, want failed when an item failed", g.State)
	}
}

func TestExpand_AGroupWhereEverythingWorkedIsCompleted(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa", "bbbbbbbbbbb")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	d.reconcileYtdlOnce(context.Background())
	for _, it := range items {
		jobs.setStatus(t, it.RequestID, k8s.JobStatus{Succeeded: 1})
	}
	d.reconcileYtdlOnce(context.Background())

	g, _ := st.GetYtdlDownload(gid)
	if g.State != "completed" {
		t.Fatalf("group = %q, want completed", g.State)
	}
	if g.FinishedAt == nil {
		t.Error("a finished group must carry when it finished")
	}
}

// FR-013d, the other way a download can sit forever: a job the orchestrator
// never starts.
func TestReconcile_AScheduledJobThatNeverAppearsDoesNotWaitForever(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())

	// The orchestrator loses the job entirely — it was created and is now gone,
	// with no terminal condition. The record must not be left believing it is
	// still starting.
	_ = jobs.DeleteJob(context.Background(), ytdl.JobName(sub.RequestID))
	for i := 0; i < 3; i++ {
		d.reconcileYtdlOnce(context.Background())
	}

	got, _ := st.GetYtdlDownload(sub.RequestID)
	if got.State == "scheduled" || got.State == "downloading" {
		t.Fatalf("state = %q after its job vanished, want it resolved to a final state", got.State)
	}
}

func TestAdmit_AGroupDoesNotOccupyASlot(t *testing.T) {
	// A group sits in `downloading` while its items run, but has no worker of
	// its own. Counting it against the limit would make a channel download
	// three at a time instead of four, with nothing on screen to explain why.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats",
		"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd", "eeeeeeeeeee")
	d.reconcileYtdlOnce(context.Background())

	running, err := st.CountYtdlRunning()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if running != 4 {
		t.Fatalf("running = %d, want the full limit of 4 — the group must not hold a slot", running)
	}
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"synodl/server/internal/config"
	"synodl/server/internal/k8s"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/synomock"
	"synodl/server/internal/ytdl"
)

// fakeJobs stands in for the orchestrator. Handlers depend on the narrow
// JobRunner interface precisely so this is possible, matching how every other
// handler here is tested against a fake syno.Client.
type fakeJobs struct {
	mu        sync.Mutex
	jobs      []k8s.Job
	created   []*k8s.Job
	deleted   []string
	listErr   error
	createErr error
	// pods and logs stand in for the worker output the reconciler reads. Keyed
	// by pod name; a request id's pod is named after its job.
	pods    []k8s.Pod
	logs    map[string]string
	logErr  error
	logReqs []string
	// nCalls counts every call the orchestrator receives, whatever it was.
	// Spec 1038 needs this: no number of watching clients may change how often
	// the cluster is asked (FR-010, SC-004), and a count is the only way to
	// assert "none" rather than "not many".
	nCalls int
}

// calls is how many times the orchestrator has been asked anything.
func (f *fakeJobs) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nCalls
}

func (f *fakeJobs) CreateJob(_ context.Context, j *k8s.Job) (*k8s.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, j)
	f.jobs = append(f.jobs, *j)
	return j, nil
}

func (f *fakeJobs) ListJobs(_ context.Context, _ string) ([]k8s.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]k8s.Job{}, f.jobs...), nil
}

func (f *fakeJobs) DeleteJob(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCalls++
	f.deleted = append(f.deleted, name)
	for i, j := range f.jobs {
		if j.Metadata.Name == name {
			f.jobs = append(f.jobs[:i], f.jobs[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeJobs) ListPods(_ context.Context, _ string) ([]k8s.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]k8s.Pod{}, f.pods...), nil
}

func (f *fakeJobs) PodLog(_ context.Context, name string, _ k8s.PodLogOptions) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nCalls++
	// Recorded so a test can assert output is read because a download is
	// running, not because someone asked for the list (FR-013f).
	f.logReqs = append(f.logReqs, name)
	if f.logErr != nil {
		return nil, f.logErr
	}
	out, ok := f.logs[name]
	if !ok {
		// A swept pod is the ordinary case, not a fault.
		return nil, &k8s.APIError{Status: 404, Message: "not found"}
	}
	return []byte(out), nil
}

// attachPod gives a request id a running pod, the way the cluster would once it
// schedules a worker. The pod carries the job's labels, so one selector finds
// both — which is how the reconciler matches them.
func (f *fakeJobs) attachPod(requestID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pods = append(f.pods, k8s.Pod{
		Metadata: k8s.ObjectMeta{
			Name:   ytdl.JobName(requestID) + "-worker",
			Labels: map[string]string{ytdl.LabelRequestID: requestID},
		},
		Status: k8s.PodStatus{Phase: "Running"},
	})
}

// emitFor appends a line of worker output for a request id's pod.
func (f *fakeJobs) emitFor(requestID, line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string]string{}
	}
	f.logs[ytdl.JobName(requestID)+"-worker"] += line + "\n"
}

// emit gives a running download some worker output to be read, the way the
// cluster would.
func (f *fakeJobs) emit(podName, output string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string]string{}
	}
	f.logs[podName] = output
	f.pods = append(f.pods, k8s.Pod{
		Metadata: k8s.ObjectMeta{Name: podName},
		Status:   k8s.PodStatus{Phase: "Running"},
	})
}

// setStatusByName drives a job addressed by its object name.
func (f *fakeJobs) setStatusByName(t *testing.T, name string, st k8s.JobStatus) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.jobs {
		if f.jobs[i].Metadata.Name == name {
			f.jobs[i].Status = st
			return
		}
	}
	t.Fatalf("no job named %q", name)
}

// attachPodFor gives a named job a running pod carrying the labels the
// reconciler matches on.
func (f *fakeJobs) attachPodFor(jobName, requestID, jobKind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs == nil {
		f.logs = map[string]string{}
	}
	f.pods = append(f.pods, k8s.Pod{
		Metadata: k8s.ObjectMeta{
			Name: jobName + "-worker",
			Labels: map[string]string{
				ytdl.LabelRequestID: requestID,
				ytdl.LabelJobKind:   jobKind,
			},
		},
		Status: k8s.PodStatus{Phase: "Running"},
	})
}

// setStatus drives a created job to a lifecycle state, the way the cluster would.
func (f *fakeJobs) setStatus(t *testing.T, requestID string, st k8s.JobStatus) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.jobs {
		if f.jobs[i].Metadata.Labels[ytdl.LabelRequestID] == requestID {
			f.jobs[i].Status = st
			return
		}
	}
	t.Fatalf("no job for request %q", requestID)
}

func ytdlCfg() config.Config {
	return config.Config{
		MaxTorrentMB: 16, LoginPerMinute: 1000, UploadMaxMB: 8,
		YtdlImage:              "jauderho/yt-dlp:2026.08.19",
		YtdlMusicClaim:         "synodl-music",
		YtdlMusicVideoClaim:    "synodl-music-video",
		YtdlUID:                1000,
		YtdlGID:                1000,
		YtdlDeadlineSeconds:    7200,
		YtdlTTLSeconds:         86400,
		YtdlMinDurationSeconds: 90,
	}
}

func newYtdlRouter(t *testing.T, jobs JobRunner, cfg config.Config) (http.Handler, *store.Store) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mock := httptest.NewServer(synomock.New().Handler())
	t.Cleanup(mock.Close)
	factory := func(base string, insecure bool) syno.Client { return syno.NewHTTPClient(mock.URL, false) }
	return NewRouter(Deps{
		Cfg: cfg, Version: "test", Stateful: true, Store: st,
		NAS: nas.New(st, factory), Jobs: jobs,
		// Pointed at a closed port with a tiny deadline: these tests must never
		// touch the network, and "the lookup failed" is the case this feature
		// has to survive anyway (spec 1034, FR-003).
		Describer: ytdl.Describer{BaseURL: "http://127.0.0.1:1", Timeout: 50 * time.Millisecond},
	}), st
}

type ytdlSubmitResp struct {
	RequestID string `json:"requestId"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
}

type ytdlListResp struct {
	Downloads []struct {
		RequestID   string `json:"requestId"`
		URL         string `json:"url"`
		Mode        string `json:"mode"`
		Scope       string `json:"scope"`
		State       string `json:"state"`
		Title       string `json:"title"`
		Uploader    string `json:"uploader"`
		SubmittedBy string `json:"submittedBy"`
		Reason      string `json:"reason"`
	} `json:"downloads"`
	Degraded bool `json:"degraded"`
}

func submit(t *testing.T, h http.Handler, auth map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, h, "POST", "/v1/ytdl", body, auth)
}

func listYtdl(t *testing.T, h http.Handler, auth map[string]string) ytdlListResp {
	t.Helper()
	rec := do(t, h, "GET", "/v1/ytdl", "", auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/ytdl = %d %s", rec.Code, rec.Body.String())
	}
	var out ytdlListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// ytdlAdmit runs one admission pass, the way the reconciler does on its clock.
// Tests call it explicitly so they stay deterministic rather than waiting on a
// ticker.
func ytdlAdmit(t *testing.T, jobs JobRunner, st *store.Store) {
	t.Helper()
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.admitQueued(context.Background())
}

// Submitting no longer creates a worker: it records the request and QUEUES it,
// and the reconciler admits it when a slot frees (spec 0013, FR-021/FR-022).
func TestYtdlSubmit_QueuesRatherThanStartingImmediately(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/zSGhyrF7YVo","mode":"music"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", rec.Code, rec.Body.String())
	}
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.RequestID == "" || out.Scope != "single" || out.Mode != "music" || out.State != "queued" {
		t.Fatalf("unexpected response: %+v", out)
	}

	// The record exists before any worker does.
	if rec, err := st.GetYtdlDownload(out.RequestID); err != nil || rec.State != "queued" {
		t.Fatalf("record = %+v (%v), want it queued", rec, err)
	}

	ytdlAdmit(t, jobs, st)
	if len(jobs.created) != 1 {
		t.Fatalf("want 1 job created after admission, got %d", len(jobs.created))
	}
	j := jobs.created[0]
	if j.Metadata.Labels[ytdl.LabelMode] != "music" {
		t.Errorf("job mode label = %q", j.Metadata.Labels[ytdl.LabelMode])
	}
	// The URL must be a discrete argv element on the created job.
	args := j.Spec.Template.Spec.Containers[0].Args
	if args[len(args)-1] != "https://youtu.be/zSGhyrF7YVo" || args[len(args)-2] != "--" {
		t.Errorf("job args do not end with `-- <url>`: %v", args)
	}
}

// FR-003: the allowlist is checked before anything happens, and nothing is
// created for a link outside it.
func TestYtdlSubmit_RejectsBlockedLinks(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://vimeo.com/1","mode":"music"}`,
		`{"url":"https://youtube.com.evil.tld/watch?v=a","mode":"music"}`,
		`{"url":"https://www.youtube.com@evil.tld/watch?v=a","mode":"music"}`,
		`{"url":"file:///etc/passwd","mode":"music"}`,
		`{"url":"","mode":"music"}`,
		`{"url":"https://youtu.be/a","mode":"bogus"}`,
		`{"url":"https://youtu.be/a"}`,
	} {
		jobs := &fakeJobs{}
		h, _ := newYtdlRouter(t, jobs, ytdlCfg())
		admin := adminAfterSetup(t, h)
		rec := submit(t, h, admin, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
		if len(jobs.created) != 0 {
			t.Errorf("%s created a job despite being refused", body)
		}
	}
}

func TestYtdlSubmit_RequiresASession(t *testing.T) {
	h, _ := newYtdlRouter(t, &fakeJobs{}, ytdlCfg())
	_ = adminAfterSetup(t, h)
	if rec := submit(t, h, nil, `{"url":"https://youtu.be/a","mode":"music"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated submit = %d, want 401", rec.Code)
	}
}

// A deployment that is not in a cluster, or has not configured the libraries,
// must say so — a 503 with an explanation, never a 500.
func TestYtdlSubmit_UnavailableWithoutAnOrchestrator(t *testing.T) {
	t.Run("no orchestrator", func(t *testing.T) {
		h, _ := newYtdlRouter(t, nil, ytdlCfg())
		admin := adminAfterSetup(t, h)
		if rec := submit(t, h, admin, `{"url":"https://youtu.be/a","mode":"music"}`); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, want 503", rec.Code)
		}
	})
	t.Run("not configured", func(t *testing.T) {
		cfg := ytdlCfg()
		cfg.YtdlImage, cfg.YtdlMusicClaim, cfg.YtdlMusicVideoClaim = "", "", ""
		h, _ := newYtdlRouter(t, &fakeJobs{}, cfg)
		admin := adminAfterSetup(t, h)
		if rec := submit(t, h, admin, `{"url":"https://youtu.be/a","mode":"music"}`); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, want 503", rec.Code)
		}
	})
	t.Run("mode whose library is not configured", func(t *testing.T) {
		cfg := ytdlCfg()
		cfg.YtdlMusicVideoClaim = ""
		h, _ := newYtdlRouter(t, &fakeJobs{}, cfg)
		admin := adminAfterSetup(t, h)
		if rec := submit(t, h, admin, `{"url":"https://youtu.be/a","mode":"music-video"}`); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, want 503", rec.Code)
		}
	})
}

// FR-023: two workers must never write the same folder concurrently.
func TestYtdlSubmit_RefusesADuplicateInFlight(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	first := submit(t, h, admin, `{"url":"https://youtu.be/dup","mode":"music"}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first submit = %d", first.Code)
	}
	// Refused while it is still only QUEUED — no job exists yet, which is
	// exactly why this check reads the store rather than the orchestrator.
	dup := submit(t, h, admin, `{"url":"https://youtu.be/dup","mode":"music"}`)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate submit = %d, want 409", dup.Code)
	}
	ytdlAdmit(t, jobs, st)
	if len(jobs.created) != 1 {
		t.Errorf("duplicate created a second job")
	}

	// The SAME link in the OTHER mode is a different download and is allowed:
	// audio and video live in different libraries.
	other := submit(t, h, admin, `{"url":"https://youtu.be/dup","mode":"music-video"}`)
	if other.Code != http.StatusAccepted {
		t.Errorf("same link in the other mode = %d, want 202", other.Code)
	}
}

func TestYtdlList_ReportsTheLifecycleStates(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)

	check := func(want string) {
		t.Helper()
		got := listYtdl(t, h, admin)
		if len(got.Downloads) != 1 {
			t.Fatalf("want 1 download, got %d", len(got.Downloads))
		}
		if got.Downloads[0].State != want {
			t.Errorf("state = %q, want %q", got.Downloads[0].State, want)
		}
	}
	// Queued until admission runs; the reconciler does this on its clock.
	check("queued")
	ytdlAdmit(t, jobs, st)
	check("scheduled")
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	// "downloading", not spec 0012's "started": the six-state model renamed it
	// so the state that carries progress says what it is (FR-013a).
	check("downloading")
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Succeeded: 1})
	check("completed")
}

// A YouTube download is not a NAS transfer, and its shape must not pretend to
// be one. Spec 0012 excluded progress along with everything else because the
// worker had no channel back to us; spec 0013 reads the worker's own output, so
// progress belongs here — but the transfer-shaped fields still do not, because
// nothing produces them and a zero-valued field is an invitation to start.
//
// Progress itself must be ABSENT rather than 0 until something is known, which
// is the same rule stated from the other side (FR-013): an unreadable log must
// not render as a download stuck at the start.
func TestYtdlList_CarriesNoTransferFields(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)

	rec := do(t, h, "GET", "/v1/ytdl", "", admin)
	var raw struct {
		Downloads []map[string]any `json:"downloads"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, forbidden := range []string{"size", "downloaded", "uploaded", "downloadSpeed", "uploadSpeed", "eta", "peers", "seeders"} {
		if _, ok := raw.Downloads[0][forbidden]; ok {
			t.Errorf("field %q must not exist in the ytdl shape — nothing produces it", forbidden)
		}
	}
	if _, ok := raw.Downloads[0]["progress"]; ok {
		t.Error("progress must be absent while nothing is known, never 0 (FR-013)")
	}
	if _, ok := raw.Downloads[0]["finishedAt"]; ok {
		t.Error("finishedAt must be absent until the download is final, never 0 (FR-033)")
	}
}

// FR-018, and the sharpest edge in the feature: a job that disappears without
// ever reporting success must NEVER be shown as completed.
func TestYtdlList_AVanishedJobIsNotCompleted(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	ytdlAdmit(t, jobs, st)

	// It fails, is observed, and is then swept by TTL.
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{
		Conditions: []k8s.JobCondition{{Type: "Failed", Status: "True", Reason: "BackoffLimitExceeded"}},
	})
	got := listYtdl(t, h, admin)
	if got.Downloads[0].State != "failed" {
		t.Fatalf("state = %q, want failed", got.Downloads[0].State)
	}
	if rec, err := st.GetYtdlDownload(sub.RequestID); err != nil || rec.State != "failed" {
		t.Fatalf("a failure must be recorded durably, got %+v (%v)", rec, err)
	}

	_ = jobs.DeleteJob(context.Background(), ytdl.JobName(sub.RequestID))
	got = listYtdl(t, h, admin)
	if len(got.Downloads) != 1 || got.Downloads[0].State != "failed" {
		t.Fatalf("a swept failure must remain visible as failed, got %+v", got.Downloads)
	}
}

// Re-listing repeatedly must not pile up duplicate failure rows: state is
// polled, so the same failed job is seen again and again.
func TestYtdlList_RepeatedObservationDoesNotDuplicate(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	ytdlAdmit(t, jobs, st)
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Failed: 1})

	for i := 0; i < 4; i++ {
		listYtdl(t, h, admin)
	}
	// Polling must not keep rewriting the record: the FIRST sighting is when it
	// failed, and a moving timestamp would make the history meaningless.
	first, err := st.GetYtdlDownload(sub.RequestID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	listYtdl(t, h, admin)
	again, _ := st.GetYtdlDownload(sub.RequestID)
	if first.FinishedAt == nil || again.FinishedAt == nil || *first.FinishedAt != *again.FinishedAt {
		t.Fatalf("re-observing moved finishedAt: %v then %v", first.FinishedAt, again.FinishedAt)
	}
	got := listYtdl(t, h, admin)
	if len(got.Downloads) != 1 {
		t.Fatalf("the same failure is listed %d times", len(got.Downloads))
	}
}

// An unreachable orchestrator must degrade, not fail: stored failures still
// come back and the client is told the live half is missing.
func TestYtdlList_DegradesWhenTheOrchestratorIsDown(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	if err := st.CreateYtdlDownload(store.YtdlDownload{
		RequestID: "old", Kind: store.YtdlKindSingle,
		SourceURL: "https://youtu.be/old", VideoID: "old", Mode: "music", Scope: "single",
		State: "failed", Reason: "the download did not complete", CreatedAt: 1757000000,
	}); err != nil {
		t.Fatal(err)
	}
	jobs.listErr = context.DeadlineExceeded

	got := listYtdl(t, h, admin)
	if !got.Degraded {
		t.Error("degraded flag should be set when the orchestrator is unreachable")
	}
	if len(got.Downloads) != 1 || got.Downloads[0].RequestID != "old" {
		t.Errorf("stored failures should still be returned, got %+v", got.Downloads)
	}
}

func TestYtdlDismiss(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	ytdlAdmit(t, jobs, st)

	// FR-005c. Spec 0012 refused this while a download was running; spec 0013
	// allows it, because with a queue and with groups of hundreds there has to
	// be a way to call off work already started. The record goes; the worker is
	// left to finish rather than stranded mid-write.
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	if r := do(t, h, "DELETE", "/v1/ytdl/"+sub.RequestID, "", admin); r.Code != http.StatusNoContent {
		t.Fatalf("dismiss while running = %d, want 204", r.Code)
	}
	if _, err := st.GetYtdlDownload(sub.RequestID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("dismiss should remove the record, got %v", err)
	}
	if len(jobs.deleted) != 0 {
		t.Errorf("a running worker was deleted mid-write: %v", jobs.deleted)
	}
	if r := do(t, h, "DELETE", "/v1/ytdl/nope", "", admin); r.Code != http.StatusNotFound {
		t.Errorf("dismissing an unknown id = %d, want 404", r.Code)
	}
}

// FR-003. The metadata lookup is decoration; it must never be the reason a
// download does not start. Every other test in this file already runs with a
// Describer pointed at a closed port, so the whole suite is this assertion —
// but state it once explicitly, because it is a promise rather than a
// side-effect.
func TestYtdlSubmit_SucceedsWhenTheLookupFails(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s; a failed lookup must not fail the download", rec.Code, rec.Body.String())
	}
	ytdlAdmit(t, jobs, st)
	if len(jobs.created) != 1 {
		t.Fatalf("want the job created anyway, got %d", len(jobs.created))
	}
	// And the row falls back rather than carrying an empty title.
	if _, ok := jobs.created[0].Metadata.Annotations[ytdl.AnnTitle]; ok {
		t.Error("an unknown title must be an ABSENT annotation, not an empty one")
	}
}

// What a successful lookup puts on the job, so the row can render it.
func TestYtdlSubmit_CarriesTheDescriptionOntoTheJob(t *testing.T) {
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"title":"A Song","author_name":"An Artist","thumbnail_url":"https://i.ytimg.com/vi/abc/hq.jpg"}`))
	}))
	defer meta.Close()

	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mock := httptest.NewServer(synomock.New().Handler())
	t.Cleanup(mock.Close)
	jobs := &fakeJobs{}
	h := NewRouter(Deps{
		Cfg: ytdlCfg(), Version: "test", Stateful: true, Store: st,
		NAS:  nas.New(st, func(base string, insecure bool) syno.Client { return syno.NewHTTPClient(mock.URL, false) }),
		Jobs: jobs, Describer: ytdl.Describer{BaseURL: meta.URL, Timeout: 2 * time.Second},
	})
	admin := adminAfterSetup(t, h)

	if rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d", rec.Code)
	}
	ytdlAdmit(t, jobs, st)
	ann := jobs.created[0].Metadata.Annotations
	if ann[ytdl.AnnTitle] != "A Song" || ann[ytdl.AnnUploader] != "An Artist" {
		t.Errorf("description not carried onto the job: %+v", ann)
	}

	got := listYtdl(t, h, admin)
	if got.Downloads[0].Title != "A Song" || got.Downloads[0].Uploader != "An Artist" {
		t.Errorf("description not surfaced on the view: %+v", got.Downloads[0])
	}
}

// Ownership (spec 0013, US1). Until this landed, every signed-in user saw every
// other user's YouTube downloads — the list filtered nothing and gated only the
// "added by" name behind admin. These tests pin the rule that NAS tasks already
// follow: your own, unless you are an admin.

// ytdlSecondUser creates a non-admin and returns their auth header.
func ytdlSecondUser(t *testing.T, h http.Handler, admin map[string]string, name string) map[string]string {
	t.Helper()
	rec := do(t, h, "POST", "/v1/users",
		`{"username":"`+name+`","password":"example-`+name+`-pw","isAdmin":false}`, admin)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create user %s = %d %s", name, rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/v1/session",
		`{"username":"`+name+`","password":"example-`+name+`-pw"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s = %d %s", name, rec.Code, rec.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	return map[string]string{"X-SynoDL-Session": login.Token}
}

func TestYtdlList_ShowsOnlyYourOwn(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	if rec := submit(t, h, admin, `{"url":"https://youtu.be/adminSong","mode":"music"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("admin submit = %d %s", rec.Code, rec.Body.String())
	}
	if rec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("bo submit = %d %s", rec.Code, rec.Body.String())
	}

	boList := listYtdl(t, h, bo)
	if len(boList.Downloads) != 1 {
		t.Fatalf("bo sees %d downloads, want only their own", len(boList.Downloads))
	}
	if !strings.Contains(boList.Downloads[0].URL, "boSong") {
		t.Fatalf("bo sees %q, want their own download", boList.Downloads[0].URL)
	}
	// And nothing about the other user's download leaks through attribution.
	if boList.Downloads[0].SubmittedBy != "" {
		t.Errorf("non-admin sees submittedBy = %q, want it withheld", boList.Downloads[0].SubmittedBy)
	}
}

func TestYtdlList_AdminSeesEveryoneAttributed(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	_ = submit(t, h, admin, `{"url":"https://youtu.be/adminSong","mode":"music"}`)
	_ = submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)

	got := listYtdl(t, h, admin)
	if len(got.Downloads) != 2 {
		t.Fatalf("admin sees %d downloads, want everyone's", len(got.Downloads))
	}
	for _, d := range got.Downloads {
		if d.SubmittedBy == "" {
			t.Errorf("admin sees %s with no submittedBy, want attribution", d.URL)
		}
	}
}

// FR-008: a refusal must not confirm that a download exists. 404 — never 403 —
// so another user's download and a request id that never existed are
// indistinguishable from outside.
func TestYtdlDismiss_AnotherUsersIsNotFound(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, admin, `{"url":"https://youtu.be/adminSong","mode":"music"}`)
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	ytdlAdmit(t, jobs, st)
	jobs.setStatus(t, out.RequestID, k8s.JobStatus{Succeeded: 1})

	theirs := do(t, h, "DELETE", "/v1/ytdl/"+out.RequestID, "", bo)
	imaginary := do(t, h, "DELETE", "/v1/ytdl/does-not-exist", "", bo)

	if theirs.Code != http.StatusNotFound {
		t.Errorf("dismissing another user's download = %d, want 404 (403 would confirm it exists)", theirs.Code)
	}
	if imaginary.Code != theirs.Code {
		t.Errorf("a nonexistent id answers %d but another user's answers %d — the two must be indistinguishable",
			imaginary.Code, theirs.Code)
	}
	if len(jobs.deleted) != 0 {
		t.Errorf("a refused dismissal still deleted %v", jobs.deleted)
	}
}

func TestYtdlDismiss_AdminMayDismissAnothersDownload(t *testing.T) {
	// FR-009a, stated positively: an admin's ability to act on any download is a
	// decision, not an accident of how FR-008's exception is phrased.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	ytdlAdmit(t, jobs, st)
	jobs.setStatus(t, out.RequestID, k8s.JobStatus{Succeeded: 1})

	if rec := do(t, h, "DELETE", "/v1/ytdl/"+out.RequestID, "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("admin dismiss = %d %s, want 204", rec.Code, rec.Body.String())
	}
}

// listYtdl2 lists without failing the test on a non-200, for the cases where a
// degraded answer is the thing under test.
func listYtdl2(t *testing.T, h http.Handler, auth map[string]string) ytdlListResp {
	t.Helper()
	rec := do(t, h, "GET", "/v1/ytdl", "", auth)
	var out ytdlListResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// Polish (spec 0013, Phase 12). Three properties that span the whole feature
// and belong to no single story.

// FR-032a: the values this feature handles that must never leave the server.
func TestYtdl_NeverLeaksSourceValuesToAClient(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")
	jobs.emitFor(id, ytdl.ProgressSentinel+" status=downloading downloaded=10 total=100")
	jobs.emitFor(id, "[download] Destination: /out/Queen/Singles/Bohemian Rhapsody.mp3")
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())

	// Whatever a worker said, what reaches a client is a state, a percentage,
	// a language and plain language — never raw output or a library path.
	body := do(t, h, "GET", "/v1/ytdl", "", admin).Body.String()
	for _, forbidden := range []string{"/out", "yt-dlp", "Destination:", ytdl.ProgressSentinel} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the list response leaks %q: %s", forbidden, body)
		}
	}
	detail := do(t, h, "GET", "/v1/ytdl/"+id, "", admin).Body.String()
	for _, forbidden := range []string{"/out", "yt-dlp", "Destination:", ytdl.ProgressSentinel} {
		if strings.Contains(detail, forbidden) {
			t.Errorf("the detail response leaks %q: %s", forbidden, detail)
		}
	}
}

// FR-032b: an infrastructure problem must stay distinguishable from a download
// that failed. Conflating them means a user retrying a link that was never
// broken, or ignoring one that is.
func TestYtdlList_OrchestratorTroubleIsNotADownloadFailing(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	jobs.listErr = context.DeadlineExceeded
	got := listYtdl(t, h, admin)

	if !got.Degraded {
		t.Error("an unreachable orchestrator must set degraded")
	}
	for _, d := range got.Downloads {
		if d.RequestID == id && d.State == "failed" {
			t.Fatal("an unreachable orchestrator reported a running download as failed")
		}
	}
}

// FR-009c: the queue is inherently cross-user, and a non-admin must learn
// nothing from it about anybody else.
func TestYtdlList_TheQueueRevealsNothingCrossUser(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	// The admin fills the queue.
	for i := 0; i < 8; i++ {
		submit(t, h, admin, `{"url":"https://youtu.be/admin`+itoa2(i)+`","mode":"music"}`)
	}
	submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	ytdlAdmit(t, jobs, st)

	rec := do(t, h, "GET", "/v1/ytdl", "", bo)
	body := rec.Body.String()
	if strings.Contains(body, "admin") {
		t.Errorf("bo's list mentions the admin: %s", body)
	}
	var got ytdlListResp
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Downloads) != 1 {
		t.Fatalf("bo sees %d downloads, want only their own", len(got.Downloads))
	}
	// No count, no position, no total — nothing that would let bo infer how much
	// other work exists.
	for _, leak := range []string{"queueLength", "position", "ahead", "total"} {
		if strings.Contains(body, leak) {
			t.Errorf("the list exposes %q, which would describe other users' work", leak)
		}
	}
}

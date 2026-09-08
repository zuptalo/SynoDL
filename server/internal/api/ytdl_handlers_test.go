package api

import (
	"context"
	"encoding/json"
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
}

func (f *fakeJobs) CreateJob(_ context.Context, j *k8s.Job) (*k8s.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]k8s.Job{}, f.jobs...), nil
}

func (f *fakeJobs) DeleteJob(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]k8s.Pod{}, f.pods...), nil
}

func (f *fakeJobs) PodLog(_ context.Context, name string, _ k8s.PodLogOptions) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

func TestYtdlSubmit_CreatesAJob(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/zSGhyrF7YVo","mode":"music"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s", rec.Code, rec.Body.String())
	}
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.RequestID == "" || out.Scope != "single" || out.Mode != "music" || out.State != "scheduled" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if len(jobs.created) != 1 {
		t.Fatalf("want 1 job created, got %d", len(jobs.created))
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
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	first := submit(t, h, admin, `{"url":"https://youtu.be/dup","mode":"music"}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first submit = %d", first.Code)
	}
	dup := submit(t, h, admin, `{"url":"https://youtu.be/dup","mode":"music"}`)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate submit = %d, want 409", dup.Code)
	}
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

func TestYtdlList_ReportsTheFourStates(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
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
	check("scheduled")
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	check("started")
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Succeeded: 1})
	check("completed")
}

// FR-017: absent from the contract, not present-and-zero, so nothing can
// quietly begin populating them.
func TestYtdlList_CarriesNoProgressFields(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)

	rec := do(t, h, "GET", "/v1/ytdl", "", admin)
	var raw struct {
		Downloads []map[string]any `json:"downloads"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	for _, forbidden := range []string{"size", "downloaded", "uploaded", "downloadSpeed", "uploadSpeed", "progress", "eta", "peers", "seeders"} {
		if _, ok := raw.Downloads[0][forbidden]; ok {
			t.Errorf("field %q must not exist in the ytdl shape (FR-017)", forbidden)
		}
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

	// It fails, is observed, and is then swept by TTL.
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{
		Conditions: []k8s.JobCondition{{Type: "Failed", Status: "True", Reason: "BackoffLimitExceeded"}},
	})
	got := listYtdl(t, h, admin)
	if got.Downloads[0].State != "failed" {
		t.Fatalf("state = %q, want failed", got.Downloads[0].State)
	}
	stored, _ := st.ListYtdlFailures()
	if len(stored) != 1 {
		t.Fatalf("a failure must be recorded durably, got %d rows", len(stored))
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
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Failed: 1})

	for i := 0; i < 4; i++ {
		listYtdl(t, h, admin)
	}
	stored, _ := st.ListYtdlFailures()
	if len(stored) != 1 {
		t.Fatalf("polling created %d failure rows, want 1", len(stored))
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
	if err := st.RecordYtdlFailure(store.YtdlFailure{
		RequestID: "old", SourceURL: "https://youtu.be/old", Mode: "music", Scope: "single",
		Reason: "the download did not complete", FailedAt: 1757000000,
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

	// While it is still running, dismissing would strand the worker.
	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Active: 1})
	if r := do(t, h, "DELETE", "/v1/ytdl/"+sub.RequestID, "", admin); r.Code != http.StatusConflict {
		t.Fatalf("dismiss while running = %d, want 409", r.Code)
	}

	jobs.setStatus(t, sub.RequestID, k8s.JobStatus{Failed: 1})
	listYtdl(t, h, admin) // observe, so the failure is recorded
	if r := do(t, h, "DELETE", "/v1/ytdl/"+sub.RequestID, "", admin); r.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d, want 204", r.Code)
	}
	stored, _ := st.ListYtdlFailures()
	if len(stored) != 0 {
		t.Errorf("dismiss should remove the stored failure, got %+v", stored)
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
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit = %d %s; a failed lookup must not fail the download", rec.Code, rec.Body.String())
	}
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
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, admin, `{"url":"https://youtu.be/adminSong","mode":"music"}`)
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
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
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	var out ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	jobs.setStatus(t, out.RequestID, k8s.JobStatus{Succeeded: 1})

	if rec := do(t, h, "DELETE", "/v1/ytdl/"+out.RequestID, "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("admin dismiss = %d %s, want 204", rec.Code, rec.Body.String())
	}
}

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
	"synodl/server/internal/musicrepair"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/synomock"
)

// repairJobs is the orchestrator fake plus the one extra call this feature needs:
// reading the server's OWN pod to learn its image.
type repairJobs struct {
	*fakeJobs
	self    *k8s.Pod
	selfErr error
	asked   []string
}

func (r *repairJobs) GetPod(_ context.Context, name string) (*k8s.Pod, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, name)
	if r.selfErr != nil {
		return nil, r.selfErr
	}
	return r.self, nil
}

func repairCfg() config.Config {
	c := ytdlCfg()
	c.MusicRepairImage = "ghcr.io/zuptalo/synodl@sha256:0123abcd"
	return c
}

// newRepairRouter is a stateful router with an orchestrator and the given config.
func newRepairRouter(t *testing.T, jobs JobRunner, cfg config.Config) (http.Handler, *store.Store) {
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
	return NewRouter(Deps{Cfg: cfg, Version: "test", Stateful: true, Store: st, NAS: nas.New(st, factory), Jobs: jobs}), st
}

type repairSnap struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Current   *struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		State     string `json:"state"`
		StartedBy string `json:"startedBy"`
		Progress  *struct {
			Phase string `json:"phase"`
			Done  int64  `json:"done"`
			Total int64  `json:"total"`
		} `json:"progress"`
	} `json:"current"`
	Plan *struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		CanApply    bool   `json:"canApply"`
		CanContinue bool   `json:"canContinue"`
		ExpiresAt   int64  `json:"expiresAt"`
		Summary     *musicrepair.Summary
	} `json:"plan"`
	Undo *struct {
		PlanID  string `json:"planId"`
		CanUndo bool   `json:"canUndo"`
	} `json:"undo"`
	History []struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		State     string `json:"state"`
		StartedBy string `json:"startedBy"`
		Headline  string `json:"headline"`
	} `json:"history"`
}

func getRepair(t *testing.T, h http.Handler, who map[string]string) repairSnap {
	t.Helper()
	rec := do(t, h, "GET", "/v1/library/repair", "", who)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/library/repair = %d %s", rec.Code, rec.Body.String())
	}
	var s repairSnap
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
	return s
}

func startCheck(t *testing.T, h http.Handler, who map[string]string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := do(t, h, "POST", "/v1/library/repair/check", "", who)
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out.ID
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Error
}

// ---- who may use it -----------------------------------------------------------

func TestRepair_OnlyAnAdminMayUseAnyOfIt(t *testing.T) {
	h, _ := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	user := makeUser(t, h, admin, "bo", "")
	calls := []struct{ method, path, body string }{
		{"GET", "/v1/library/repair", ""},
		{"POST", "/v1/library/repair/check", ""},
		{"POST", "/v1/library/repair/apply", `{"planId":"20260929T071341Z-73c844","snapshotAck":true}`},
		{"POST", "/v1/library/repair/undo", `{"planId":"20260929T071341Z-73c844"}`},
	}
	for _, c := range calls {
		if rec := do(t, h, c.method, c.path, c.body, user); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a regular user = %d, want 403", c.method, c.path, rec.Code)
		}
		if rec := do(t, h, c.method, c.path, c.body, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymously = %d, want 401", c.method, c.path, rec.Code)
		}
	}
}

// ---- when it cannot run --------------------------------------------------------

func TestRepair_UnavailableSaysWhyAndChangesNothingElse(t *testing.T) {
	cases := []struct {
		name   string
		jobs   JobRunner
		mod    func(*config.Config)
		reason string
	}{
		{"no cluster access", nil, func(*config.Config) {}, "not_configured"},
		{"no music library", &fakeJobs{}, func(c *config.Config) { c.YtdlMusicClaim = "" }, "not_configured"},
		{"no worker image", &fakeJobs{}, func(c *config.Config) { c.YtdlImage = "" }, "not_configured"},
		{"the server cannot tell which image it is running", &fakeJobs{}, func(c *config.Config) { c.MusicRepairImage = "" }, "no_image"},
	}
	for _, tc := range cases {
		cfg := repairCfg()
		tc.mod(&cfg)
		h, _ := newRepairRouter(t, tc.jobs, cfg)
		admin := adminAfterSetup(t, h)
		s := getRepair(t, h, admin)
		if s.Available || s.Reason != tc.reason {
			t.Errorf("%s: available=%v reason=%q, want unavailable/%s", tc.name, s.Available, s.Reason, tc.reason)
		}
		if s.Current != nil || s.Plan != nil {
			t.Errorf("%s: an unavailable snapshot carried data: %+v", tc.name, s)
		}
		if rec := do(t, h, "POST", "/v1/library/repair/check", "", admin); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "unavailable" {
			t.Errorf("%s: start = %d %s, want 503 unavailable", tc.name, rec.Code, rec.Body.String())
		}
		// The rest of the app is untouched.
		if rec := do(t, h, "GET", "/v1/library/music", "", admin); rec.Code != http.StatusOK {
			t.Errorf("%s: an unrelated route = %d", tc.name, rec.Code)
		}
	}
}

func TestRepair_TheServerLearnsItsOwnImageFromItsOwnPod(t *testing.T) {
	t.Setenv("HOSTNAME", "synodl-56f8c7f85b-q98vl")
	cfg := repairCfg()
	cfg.MusicRepairImage = ""
	jobs := &repairJobs{fakeJobs: &fakeJobs{}, self: &k8s.Pod{
		Spec: k8s.PodView{Containers: []k8s.ContainerView{{Name: "synodl", Image: "ghcr.io/zuptalo/synodl:latest"}}},
		Status: k8s.PodStatus{ContainerStatuses: []k8s.ContainerStatus{
			{Name: "synodl", Image: "ghcr.io/zuptalo/synodl:latest", ImageID: "ghcr.io/zuptalo/synodl@sha256:feedbeef"}}},
	}}
	h, _ := newRepairRouter(t, jobs, cfg)
	admin := adminAfterSetup(t, h)
	if s := getRepair(t, h, admin); !s.Available {
		t.Fatalf("snapshot = %+v, want available once the image is known", s)
	}
	if len(jobs.asked) == 0 || jobs.asked[0] != "synodl-56f8c7f85b-q98vl" {
		t.Fatalf("asked for pod %v, want the server's own (HOSTNAME)", jobs.asked)
	}
	if rec, _ := startCheck(t, h, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	got := jobs.created[0].Spec.Template.Spec.InitContainers[0].Image
	if got != "ghcr.io/zuptalo/synodl@sha256:feedbeef" {
		t.Errorf("init image = %q, want the digest the server is actually running", got)
	}
}

func TestRepair_FallsBackToTheConfiguredImageWhenTheRuntimeReportsNoDigest(t *testing.T) {
	t.Setenv("HOSTNAME", "synodl-x")
	cfg := repairCfg()
	cfg.MusicRepairImage = ""
	jobs := &repairJobs{fakeJobs: &fakeJobs{}, self: &k8s.Pod{
		Spec:   k8s.PodView{Containers: []k8s.ContainerView{{Name: "synodl", Image: "ghcr.io/zuptalo/synodl:0.20.1"}}},
		Status: k8s.PodStatus{ContainerStatuses: []k8s.ContainerStatus{{Name: "synodl", Image: "ghcr.io/zuptalo/synodl:0.20.1"}}},
	}}
	h, _ := newRepairRouter(t, jobs, cfg)
	admin := adminAfterSetup(t, h)
	startCheck(t, h, admin)
	if got := jobs.created[0].Spec.Template.Spec.InitContainers[0].Image; got != "ghcr.io/zuptalo/synodl:0.20.1" {
		t.Errorf("init image = %q", got)
	}
}

func TestRepair_AnUnreadableOwnPodMeansUnavailableNotBroken(t *testing.T) {
	t.Setenv("HOSTNAME", "synodl-x")
	cfg := repairCfg()
	cfg.MusicRepairImage = ""
	jobs := &repairJobs{fakeJobs: &fakeJobs{}, selfErr: &k8s.APIError{Status: 403, Message: "forbidden"}}
	h, _ := newRepairRouter(t, jobs, cfg)
	admin := adminAfterSetup(t, h)
	if s := getRepair(t, h, admin); s.Available || s.Reason != "no_image" {
		t.Errorf("snapshot = %+v, want unavailable/no_image", s)
	}
}

// ---- starting a check -----------------------------------------------------------

func TestRepair_EmptyWhenNothingHasRun(t *testing.T) {
	h, _ := newRepairRouter(t, &fakeJobs{}, repairCfg())
	admin := adminAfterSetup(t, h)
	s := getRepair(t, h, admin)
	if !s.Available || s.Current != nil || s.Plan != nil || s.Undo != nil || len(s.History) != 0 {
		t.Errorf("snapshot = %+v, want available and empty", s)
	}
}

func TestRepair_StartingACheckCreatesExactlyOneWorkerAndARecord(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	rec, id := startCheck(t, h, admin)
	if rec.Code != http.StatusAccepted || id == "" {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	if len(jobs.created) != 1 {
		t.Fatalf("jobs created = %d, want 1", len(jobs.created))
	}
	j := jobs.created[0]
	if j.Metadata.Labels["synodl.io/kind"] != "music-repair" || j.Metadata.Name != musicrepair.JobName(id) {
		t.Errorf("job = %s %v", j.Metadata.Name, j.Metadata.Labels)
	}
	w := j.Spec.Template.Spec.Containers[0]
	if strings.Join(w.Args, " ") != "plan --library /library" || w.Image != "jauderho/yt-dlp:2026.08.19" {
		t.Errorf("worker = %s %v", w.Image, w.Args)
	}
	if j.Spec.Template.Spec.InitContainers[0].Image != "ghcr.io/zuptalo/synodl@sha256:0123abcd" {
		t.Errorf("init image = %q", j.Spec.Template.Spec.InitContainers[0].Image)
	}
	row, err := st.GetMusicRepair(id)
	if err != nil || row.State != "running" || row.Kind != "check" || row.UserName == "" {
		t.Errorf("record = %+v %v", row, err)
	}
	s := getRepair(t, h, admin)
	if s.Current == nil || s.Current.ID != id || s.Current.State != "running" || s.Current.Kind != "check" {
		t.Errorf("snapshot current = %+v", s.Current)
	}
}

func TestRepair_ASecondRunIsRefusedAndSaysWhoIsRunning(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	startCheck(t, h, admin)
	rec, _ := startCheck(t, h, admin)
	if rec.Code != http.StatusConflict || errCode(t, rec) != "busy" {
		t.Fatalf("second start = %d %s, want 409 busy", rec.Code, rec.Body.String())
	}
	var b struct {
		StartedBy string `json:"startedBy"`
		StartedAt int64  `json:"startedAt"`
		Source    string `json:"source"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.StartedBy == "" || b.StartedAt == 0 || b.Source != "settings" {
		t.Errorf("busy body = %s, want who, when and the source", rec.Body.String())
	}
	if len(jobs.created) != 1 {
		t.Errorf("jobs created = %d, want still 1", len(jobs.created))
	}
}

func TestRepair_SimultaneousRequestsStartExactlyOneRun(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := do(t, h, "POST", "/v1/library/repair/check", "", admin)
			mu.Lock()
			codes[rec.Code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[http.StatusAccepted] != 1 || codes[http.StatusConflict] != 11 {
		t.Fatalf("responses = %v, want exactly one 202 and eleven 409", codes)
	}
	if len(jobs.created) != 1 {
		t.Errorf("jobs created = %d, want 1 (SC-003)", len(jobs.created))
	}
}

func TestRepair_ARunStartedFromTheCommandLineBlocksAndIsNamedAsSuch(t *testing.T) {
	jobs := &fakeJobs{}
	// scripts/music-repair.sh labels its Job with the shared name only.
	jobs.jobs = append(jobs.jobs, k8s.Job{
		Metadata: k8s.ObjectMeta{Name: "music-repair-plan-20260929070935",
			Labels: map[string]string{"app.kubernetes.io/name": "synodl-music-repair"}},
		Status: k8s.JobStatus{Active: 1},
	})
	h, st := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	rec, _ := startCheck(t, h, admin)
	if rec.Code != http.StatusConflict || errCode(t, rec) != "busy" || !strings.Contains(rec.Body.String(), `"command_line"`) {
		t.Fatalf("start = %d %s, want 409 busy from the command line", rec.Code, rec.Body.String())
	}
	if len(jobs.created) != 0 {
		t.Error("a Job was created next to a running command-line repair")
	}
	if r, _ := st.RunningMusicRepair(); r != nil {
		t.Error("a refused request left the slot taken")
	}
}

func TestRepair_AFinishedCommandLineJobDoesNotBlock(t *testing.T) {
	jobs := &fakeJobs{}
	jobs.jobs = append(jobs.jobs, k8s.Job{
		Metadata: k8s.ObjectMeta{Name: "music-repair-plan-old", Labels: map[string]string{"app.kubernetes.io/name": "synodl-music-repair"}},
		Status:   k8s.JobStatus{Succeeded: 1, Conditions: []k8s.JobCondition{{Type: "Complete", Status: "True"}}},
	})
	h, _ := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	if rec, _ := startCheck(t, h, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRepair_IfTheJobCannotBeCreatedTheSlotIsReleased(t *testing.T) {
	jobs := &fakeJobs{createErr: &k8s.APIError{Status: 500, Message: "boom"}}
	h, st := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	rec, id := startCheck(t, h, admin)
	if rec.Code != http.StatusBadGateway || errCode(t, rec) != "start_failed" {
		t.Fatalf("start = %d %s, want 502 start_failed", rec.Code, rec.Body.String())
	}
	if id != "" {
		t.Errorf("a run id %q was handed out for a run that never started", id)
	}
	if r, _ := st.RunningMusicRepair(); r != nil {
		t.Fatal("the slot is still held by a run that never started")
	}
	jobs.mu.Lock()
	jobs.createErr = nil
	jobs.mu.Unlock()
	if rec, _ := startCheck(t, h, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("a retry after the outage = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("the orchestrator's error text leaked to the client")
	}
}

// ---- what the client is allowed to see ------------------------------------------

func TestRepair_ProgressIsTheParsedShapeAndNeverTheRawLog(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newRepairRouter(t, jobs, repairCfg())
	admin := adminAfterSetup(t, h)
	_, id := startCheck(t, h, admin)

	pod := k8s.Pod{Metadata: k8s.ObjectMeta{Name: "pod-" + id, Labels: map[string]string{musicrepair.LabelRepairID: id}},
		Status: k8s.PodStatus{Phase: "Running"}}
	jobs.pods = append(jobs.pods, pod)
	jobs.logs = map[string]string{"pod-" + id: "[plan] looked up 1275/4017\n" +
		"SECRET-HUMAN-LINE /Volumes/private/path\n" +
		"@@synodl {\"event\":\"progress\",\"phase\":\"lookup\",\"done\":1275,\"total\":4017,\"leak\":\"SECRET-FIELD\"}\n"}

	rec := do(t, h, "GET", "/v1/library/repair", "", admin)
	body := rec.Body.String()
	for _, forbidden := range []string{"SECRET-HUMAN-LINE", "SECRET-FIELD", "/Volumes/private", "@@synodl"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the response contains %q: worker output must be parsed into a fixed shape, never forwarded (FR-016)", forbidden)
		}
	}
	var s repairSnap
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Current == nil || s.Current.Progress == nil || s.Current.Progress.Phase != "lookup" ||
		s.Current.Progress.Done != 1275 || s.Current.Progress.Total != 4017 {
		t.Errorf("current = %+v, want the parsed progress", s.Current)
	}
}

// keep the compiler honest about imports used only by later phases' tests
var _ = time.Second

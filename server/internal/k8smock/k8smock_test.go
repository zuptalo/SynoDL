package k8smock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synodl/server/internal/k8s"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(0).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func createJob(t *testing.T, srv *httptest.Server, name, mode, requestID string) {
	t.Helper()
	j := k8s.Job{Metadata: k8s.ObjectMeta{
		Name: name,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "synodl",
			"synodl.io/kind":               "ytdl",
			"synodl.io/mode":               mode,
			"synodl.io/request-id":         requestID,
		},
	}}
	body, _ := json.Marshal(j)
	resp, err := http.Post(srv.URL+"/apis/batch/v1/namespaces/synodl/jobs", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
}

func list(t *testing.T, srv *httptest.Server, selector string) []k8s.Job {
	t.Helper()
	resp, err := http.Get(srv.URL + "/apis/batch/v1/namespaces/synodl/jobs?labelSelector=" + selector)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out k8s.JobList
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Items
}

func control(t *testing.T, srv *httptest.Server, name, action string) int {
	t.Helper()
	resp, err := http.Post(srv.URL+"/__mock/jobs/"+name+"/"+action, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestCreateAndSelect(t *testing.T) {
	srv := newTestServer(t)
	createJob(t, srv, "synodl-ytdl-a", "music", "a")
	createJob(t, srv, "synodl-ytdl-b", "music-video", "b")
	createJob(t, srv, "other", "music", "c")

	// The selector must actually filter, or the real client's guarantee that it
	// never touches a Job it did not create would go untested.
	if got := list(t, srv, "app.kubernetes.io/managed-by=synodl,synodl.io/kind=ytdl"); len(got) != 3 {
		t.Errorf("managed-by selector matched %d, want 3", len(got))
	}
	if got := list(t, srv, "synodl.io/mode=music"); len(got) != 2 {
		t.Errorf("mode selector matched %d, want 2", len(got))
	}
	if got := list(t, srv, "synodl.io/mode=nope"); len(got) != 0 {
		t.Errorf("unmatched selector returned %d jobs", len(got))
	}
}

func TestLifecycleControls(t *testing.T) {
	srv := newTestServer(t)
	createJob(t, srv, "synodl-ytdl-a", "music", "req-a")

	stateOf := func() k8s.JobStatus {
		jobs := list(t, srv, "")
		for _, j := range jobs {
			if j.Metadata.Name == "synodl-ytdl-a" {
				return j.Status
			}
		}
		t.Fatal("job disappeared")
		return k8s.JobStatus{}
	}

	if control(t, srv, "synodl-ytdl-a", "start") != http.StatusOK || stateOf().Active != 1 {
		t.Error("start should mark the job active")
	}
	if control(t, srv, "synodl-ytdl-a", "fail") != http.StatusOK {
		t.Fatal("fail control")
	}
	st := stateOf()
	if st.Failed != 1 || st.Conditions[0].Type != "Failed" {
		t.Errorf("fail should set a Failed condition, got %+v", st)
	}
	if control(t, srv, "synodl-ytdl-a", "deadline") != http.StatusOK {
		t.Fatal("deadline control")
	}
	if reason := stateOf().Conditions[0].Reason; reason != "DeadlineExceeded" {
		t.Errorf("deadline reason = %q", reason)
	}
}

// The control that matters most: a job disappearing WITHOUT a terminal
// condition, which is how we prove a missing job is never read as a success.
func TestVanish(t *testing.T) {
	srv := newTestServer(t)
	createJob(t, srv, "synodl-ytdl-a", "music", "req-a")
	if control(t, srv, "synodl-ytdl-a", "vanish") != http.StatusOK {
		t.Fatal("vanish control")
	}
	if got := list(t, srv, ""); len(got) != 0 {
		t.Errorf("job survived vanish: %+v", got)
	}
}

// Controls address a job by its name OR by the request id it carries, so a
// caller can use whichever it happens to hold.
func TestControlByRequestID(t *testing.T) {
	srv := newTestServer(t)
	createJob(t, srv, "synodl-ytdl-xyz", "music", "req-xyz")
	if control(t, srv, "req-xyz", "succeed") != http.StatusOK {
		t.Fatal("control by request id should work")
	}
	if list(t, srv, "")[0].Status.Succeeded != 1 {
		t.Error("succeed did not apply")
	}
	if control(t, srv, "no-such-thing", "start") != http.StatusNotFound {
		t.Error("an unknown name should 404")
	}
}

func TestDeleteAndReset(t *testing.T) {
	srv := newTestServer(t)
	createJob(t, srv, "synodl-ytdl-a", "music", "a")

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/apis/batch/v1/namespaces/synodl/jobs/synodl-ytdl-a", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("delete = %d", resp.StatusCode)
	}
	if len(list(t, srv, "")) != 0 {
		t.Error("job survived delete")
	}

	createJob(t, srv, "synodl-ytdl-b", "music", "b")
	r2, _ := http.Post(srv.URL+"/__mock/reset", "application/json", nil)
	r2.Body.Close()
	if len(list(t, srv, "")) != 0 {
		t.Error("reset should clear every job")
	}
}

// Pods and their output (spec 0013). The reason these are tested here rather
// than faked at the Go boundary is the same reason the package exists at all:
// the wire format and the selector are where the bugs live.

func TestListPodsAndLog(t *testing.T) {
	srv := New(0)
	h := srv.Handler()

	createTestJob(t, h, "synodl-ytdl-abc", map[string]string{
		"app.kubernetes.io/managed-by": "synodl",
		"synodl.io/kind":               "ytdl",
	})

	// A pod exists for the job and carries its labels, so one selector finds both.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/synodl/pods?labelSelector=synodl.io/kind=ytdl", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list pods = %d, want 200", rec.Code)
	}
	var pods k8s.PodList
	if err := json.NewDecoder(rec.Body).Decode(&pods); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("pods = %d, want 1", len(pods.Items))
	}
	podName := pods.Items[0].Metadata.Name

	// A pod with no output yet is a 404, which is an ordinary outcome.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/synodl/pods/"+podName+"/log", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("log before emit = %d, want 404", rec.Code)
	}

	// emit gives it something to say.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__mock/jobs/synodl-ytdl-abc/emit", strings.NewReader("hello")))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("emit = %d, want 204", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/synodl/pods/"+podName+"/log", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("log = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain — the real endpoint is not JSON", got)
	}
	if rec.Body.String() != "hello\n" {
		t.Errorf("body = %q, want the emitted line", rec.Body.String())
	}
}

func TestPodLog_TailLines(t *testing.T) {
	srv := New(0)
	h := srv.Handler()
	createTestJob(t, h, "synodl-ytdl-tail", map[string]string{"synodl.io/kind": "ytdl"})
	for _, l := range []string{"one", "two", "three"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__mock/jobs/synodl-ytdl-tail/emit", strings.NewReader(l)))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/synodl/pods/synodl-ytdl-tail-worker/log?tailLines=2", nil))
	if rec.Body.String() != "two\nthree\n" {
		t.Fatalf("body = %q, want only the last two lines", rec.Body.String())
	}
}

func TestDeleteJob_SweepsPodOutput(t *testing.T) {
	// A swept Job takes its output with it. This is why facts that must outlive
	// the worker are captured while it still exists (FR-013g) — and a test that
	// did not reproduce the sweep would let that bug through.
	srv := New(0)
	h := srv.Handler()
	createTestJob(t, h, "synodl-ytdl-gone", map[string]string{"synodl.io/kind": "ytdl"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/__mock/jobs/synodl-ytdl-gone/emit", strings.NewReader("progress")))

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/apis/batch/v1/namespaces/synodl/jobs/synodl-ytdl-gone", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/synodl/pods/synodl-ytdl-gone-worker/log", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("log after sweep = %d, want 404", rec.Code)
	}
}

// createTestJob posts a minimal Job with the given labels.
func createTestJob(t *testing.T, h http.Handler, name string, labels map[string]string) {
	t.Helper()
	body, _ := json.Marshal(k8s.Job{Metadata: k8s.ObjectMeta{Name: name, Namespace: "synodl", Labels: labels}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/apis/batch/v1/namespaces/synodl/jobs", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create job = %d, want 201", rec.Code)
	}
}

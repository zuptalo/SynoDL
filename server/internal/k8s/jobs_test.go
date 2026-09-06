package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{Host: srv.URL, Token: "sekret-token", Namespace: "synodl"})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestCreateJob(t *testing.T) {
	var gotPath, gotAuth, gotCT string
	var gotBody Job
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotCT = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Job{Metadata: ObjectMeta{Name: "synodl-ytdl-abc"}})
	})

	in := &Job{Metadata: ObjectMeta{Name: "synodl-ytdl-abc", Labels: map[string]string{"a": "b"}}}
	out, err := c.CreateJob(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if want := "/apis/batch/v1/namespaces/synodl/jobs"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotAuth != "Bearer sekret-token" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotBody.Metadata.Name != "synodl-ytdl-abc" || gotBody.Metadata.Labels["a"] != "b" {
		t.Errorf("body not round-tripped: %+v", gotBody.Metadata)
	}
	if out.Metadata.Name != "synodl-ytdl-abc" {
		t.Errorf("returned job = %+v", out.Metadata)
	}
}

func TestListJobs_UsesLabelSelector(t *testing.T) {
	var gotSelector, gotPath string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSelector = r.URL.Query().Get("labelSelector")
		_ = json.NewEncoder(w).Encode(JobList{Items: []Job{
			{Metadata: ObjectMeta{Name: "one"}, Status: JobStatus{Active: 1}},
			{Metadata: ObjectMeta{Name: "two"}, Status: JobStatus{Succeeded: 1}},
		}})
	})

	jobs, err := c.ListJobs(context.Background(), "app.kubernetes.io/managed-by=synodl,synodl.io/kind=ytdl")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if want := "/apis/batch/v1/namespaces/synodl/jobs"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotSelector != "app.kubernetes.io/managed-by=synodl,synodl.io/kind=ytdl" {
		t.Errorf("selector = %q", gotSelector)
	}
	if len(jobs) != 2 || jobs[0].Metadata.Name != "one" || jobs[1].Status.Succeeded != 1 {
		t.Errorf("jobs = %+v", jobs)
	}
}

func TestDeleteJob_RemovesPodsToo(t *testing.T) {
	var gotMethod, gotPath, gotPolicy string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotPolicy = r.URL.Query().Get("propagationPolicy")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.DeleteJob(context.Background(), "synodl-ytdl-abc"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q", gotMethod)
	}
	if want := "/apis/batch/v1/namespaces/synodl/jobs/synodl-ytdl-abc"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	// Without this the Job goes but its pod is orphaned.
	if gotPolicy != "Background" {
		t.Errorf("propagationPolicy = %q, want Background", gotPolicy)
	}
}

func TestDeleteJob_NotFoundIsNotAnError(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"jobs.batch not found"}`))
	})
	// Deleting something already swept by TTL is the normal case, not a fault.
	if err := c.DeleteJob(context.Background(), "gone"); err != nil {
		t.Errorf("deleting an absent job should succeed, got %v", err)
	}
}

// Principle III: a credential must never reach a log line or an error payload.
func TestErrors_NeverLeakTheToken(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"jobs is forbidden"}`))
	})
	_, err := c.CreateJob(context.Background(), &Job{Metadata: ObjectMeta{Name: "x"}})
	if err == nil {
		t.Fatal("want an error for 403")
	}
	if strings.Contains(err.Error(), "sekret-token") {
		t.Fatalf("error leaks the bearer token: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should name the status, got %v", err)
	}
}

func TestListJobs_MalformedBody(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items": not-json`))
	})
	if _, err := c.ListJobs(context.Background(), "x=y"); err == nil {
		t.Error("want an error for a malformed body")
	}
}

func TestCreateJob_ConflictIsRecognisable(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"already exists"}`))
	})
	_, err := c.CreateJob(context.Background(), &Job{Metadata: ObjectMeta{Name: "x"}})
	if !IsConflict(err) {
		t.Errorf("want a recognisable conflict, got %v", err)
	}
}

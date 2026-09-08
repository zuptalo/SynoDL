// Package k8smock is a fake Kubernetes Jobs API: enough of the surface for
// SynoDL's own client, plus /__mock/* controls to drive a Job's lifecycle on
// demand.
//
// It exists for the same reason cmd/synomock does. The Domain Constraints say
// local dev and the e2e suite must never require real hardware, and the cluster
// API is now a dependency in exactly the way DSM already was. Faking only at
// the Go interface boundary would leave the wire format, the label selector,
// and the lifecycle-to-state mapping untested — which is precisely where this
// feature's bugs would live.
//
// It NEVER downloads anything. It records the Job it was handed and lets a test
// say what happens to it next.
package k8smock

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/k8s"
)

// Server holds the fake cluster's Jobs.
type Server struct {
	mu   sync.Mutex
	jobs map[string]*k8s.Job
	// logs is each job's pod output, keyed by POD name. A real cluster names a
	// Job's pod after the Job plus a random suffix; this mock uses a fixed
	// suffix so a test can address it, while still making the caller go through
	// a pod LIST to find it — which is what keeps the selector under test
	// rather than assumed (spec 0013).
	logs map[string]string

	// autoAdvance makes a created Job walk scheduled → started → completed on
	// its own. Off by default so tests stay deterministic; `make start` turns it
	// on so a developer can watch a download progress without curling controls.
	autoAdvance time.Duration
}

func New(autoAdvance time.Duration) *Server {
	return &Server{jobs: map[string]*k8s.Job{}, logs: map[string]string{}, autoAdvance: autoAdvance}
}

// podNameFor is how this mock names a Job's pod. A real cluster appends a random
// suffix; the shape is what matters, not the randomness.
func podNameFor(jobName string) string { return jobName + "-worker" }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// The real API surface SynoDL uses — nothing more.
	mux.HandleFunc("POST /apis/batch/v1/namespaces/{ns}/jobs", s.createJob)
	mux.HandleFunc("GET /apis/batch/v1/namespaces/{ns}/jobs", s.listJobs)
	mux.HandleFunc("DELETE /apis/batch/v1/namespaces/{ns}/jobs/{name}", s.deleteJob)
	// Pods and their output (spec 0013). Progress, and whether a lyrics file was
	// written, exist only here — so faking them at the Go boundary would leave
	// the log read, the pod selector and the text/plain response untested, which
	// is exactly where this feature's bugs would live.
	mux.HandleFunc("GET /api/v1/namespaces/{ns}/pods", s.listPods)
	mux.HandleFunc("GET /api/v1/namespaces/{ns}/pods/{name}/log", s.podLog)

	// Controls, mirroring synomock's /__mock/* convention.
	mux.HandleFunc("POST /__mock/jobs/{name}/start", s.control("start"))
	mux.HandleFunc("POST /__mock/jobs/{name}/succeed", s.control("succeed"))
	mux.HandleFunc("POST /__mock/jobs/{name}/fail", s.control("fail"))
	mux.HandleFunc("POST /__mock/jobs/{name}/deadline", s.control("deadline"))
	mux.HandleFunc("POST /__mock/jobs/{name}/vanish", s.control("vanish"))
	// emit appends lines to a job's pod output, so a test can drive a progress
	// bar, a lyrics line, or an expansion listing deterministically.
	mux.HandleFunc("POST /__mock/jobs/{name}/emit", s.emit)
	mux.HandleFunc("POST /__mock/reset", s.reset)

	// The feature's OTHER external dependency: the "what is this link?" lookup
	// (spec 1034). It lives here rather than in its own binary because this mock
	// already stands in for everything outside the cluster that these downloads
	// touch, and a second process for one endpoint would be more moving parts
	// than the thing it replaces.
	mux.HandleFunc("GET /oembed", s.oembed)
	mux.HandleFunc("GET /__mock/jobs", s.listJobs)
	return mux
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var j k8s.Job
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, `{"message":"bad job"}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if _, exists := s.jobs[j.Metadata.Name]; exists {
		s.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"already exists"}`))
		return
	}
	s.jobs[j.Metadata.Name] = &j
	name, delay := j.Metadata.Name, s.autoAdvance
	s.mu.Unlock()

	slog.Info("k8smock: job created", "name", name, "mode", j.Metadata.Labels["synodl.io/mode"])
	if delay > 0 {
		go s.advance(name, delay)
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(j)
}

// advance walks a job through its lifecycle, for hand-testing in dev.
func (s *Server) advance(name string, delay time.Duration) {
	time.Sleep(delay)
	s.apply(name, "start")
	time.Sleep(delay)
	s.apply(name, "succeed")
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	selector := r.URL.Query().Get("labelSelector")
	s.mu.Lock()
	defer s.mu.Unlock()

	out := k8s.JobList{Items: []k8s.Job{}}
	for _, j := range s.jobs {
		if matches(j.Metadata.Labels, selector) {
			out.Items = append(out.Items, *j)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// matches implements the equality-only label selector SynoDL uses.
func matches(labels map[string]string, selector string) bool {
	if strings.TrimSpace(selector) == "" {
		return true
	}
	for _, clause := range strings.Split(selector, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(clause), "=")
		if !ok {
			continue
		}
		if labels[k] != v {
			return false
		}
	}
	return true
}

// listPods returns one pod per job matching the selector. The pod carries the
// job's labels, exactly as the real thing does — BuildJob puts them on the pod
// template for this reason.
func (s *Server) listPods(w http.ResponseWriter, r *http.Request) {
	selector := r.URL.Query().Get("labelSelector")
	s.mu.Lock()
	defer s.mu.Unlock()

	out := k8s.PodList{Items: []k8s.Pod{}}
	for name, j := range s.jobs {
		if !matches(j.Metadata.Labels, selector) {
			continue
		}
		phase := "Pending"
		switch {
		case j.Status.Succeeded > 0:
			phase = "Succeeded"
		case j.Status.Failed > 0:
			phase = "Failed"
		case j.Status.Active > 0:
			phase = "Running"
		}
		out.Items = append(out.Items, k8s.Pod{
			Metadata: k8s.ObjectMeta{
				Name:      podNameFor(name),
				Namespace: j.Metadata.Namespace,
				Labels:    j.Metadata.Labels,
			},
			Status: k8s.PodStatus{Phase: phase},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// podLog answers text/plain, like the real endpoint — NOT JSON. That difference
// is the reason SynoDL's client needs a separate method, so the mock must
// reproduce it rather than smooth it over.
func (s *Server) podLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	out, ok := s.logs[name]
	s.mu.Unlock()
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"pod not found"}`))
		return
	}
	if n := r.URL.Query().Get("tailLines"); n != "" {
		if lines, err := strconv.Atoi(n); err == nil && lines > 0 {
			all := strings.Split(strings.TrimRight(out, "\n"), "\n")
			if len(all) > lines {
				all = all[len(all)-lines:]
			}
			out = strings.Join(all, "\n") + "\n"
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, out)
}

// emit appends a line of worker output to a job's pod.
func (s *Server) emit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[name]; !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no such job"}`))
		return
	}
	line := string(body)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	s.logs[podNameFor(name)] += line
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	_, ok := s.jobs[name]
	delete(s.jobs, name)
	// A swept Job takes its pod's output with it — which is precisely why facts
	// that must outlive the worker are captured while it still exists (FR-013g).
	delete(s.logs, podNameFor(name))
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
		return
	}
	_, _ = w.Write([]byte(`{"status":"Success"}`))
}

func (s *Server) control(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.apply(r.PathValue("name"), action) {
			http.Error(w, `{"message":"no such job"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

// apply drives one Job to a new lifecycle state. The name may be the Job's own
// name or the request id it carries, so a test can use whichever it has.
func (s *Server) apply(name, action string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	j := s.jobs[name]
	if j == nil {
		for _, candidate := range s.jobs {
			if candidate.Metadata.Labels["synodl.io/request-id"] == name {
				j = candidate
				break
			}
		}
	}
	if j == nil {
		return false
	}

	switch action {
	case "start":
		j.Status = k8s.JobStatus{Active: 1}
	case "succeed":
		j.Status = k8s.JobStatus{Succeeded: 1, Conditions: []k8s.JobCondition{{Type: "Complete", Status: "True"}}}
	case "fail":
		j.Status = k8s.JobStatus{Failed: 1, Conditions: []k8s.JobCondition{
			{Type: "Failed", Status: "True", Reason: "BackoffLimitExceeded"}}}
	case "deadline":
		j.Status = k8s.JobStatus{Failed: 1, Conditions: []k8s.JobCondition{
			{Type: "Failed", Status: "True", Reason: "DeadlineExceeded"}}}
	case "vanish":
		// Disappears WITHOUT a terminal condition — the case that proves a
		// missing job is never reported as a success.
		delete(s.jobs, j.Metadata.Name)
	}
	return true
}

// oembed answers the metadata lookup with a deterministic document derived from
// the link, so a test can assert the row shows a TITLE rather than a URL without
// depending on what YouTube happens to publish today.
func (s *Server) oembed(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		http.Error(w, `{"message":"no url"}`, http.StatusBadRequest)
		return
	}
	// A channel has no oEmbed document in reality, so the mock has none either —
	// that fallback path is worth exercising rather than papering over.
	if strings.Contains(raw, "/@") || strings.Contains(raw, "/channel/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := raw[strings.LastIndex(raw, "/")+1:]
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"title":"Mock Track ` + id + `","author_name":"Mock Artist",` +
		`"thumbnail_url":"https://i.ytimg.com/vi/` + id + `/hqdefault.jpg"}`))
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.jobs = map[string]*k8s.Job{}
	s.logs = map[string]string{}
	s.mu.Unlock()
	_, _ = w.Write([]byte(`{"ok":true}`))
}

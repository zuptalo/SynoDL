package k8s

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The pod calls exist for one reason: a worker's own output is the only place
// progress and companion-file facts exist (spec 0013). These tests pin the wire
// shape, because that — not the Go signature — is where the bugs live.

func podClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{Host: srv.URL, Namespace: "synodl", Token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestListPods_UsesSelectorAndNamespace(t *testing.T) {
	var gotPath, gotSelector, gotAuth string
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotSelector, gotAuth = r.URL.Path, r.URL.Query().Get("labelSelector"), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"p1","labels":{"synodl.io/kind":"ytdl"}},"status":{"phase":"Running"}}]}`))
	}))

	pods, err := c.ListPods(context.Background(), "synodl.io/kind=ytdl")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if gotPath != "/api/v1/namespaces/synodl/pods" {
		t.Errorf("path = %q, want the core-group pods path in our own namespace", gotPath)
	}
	if gotSelector != "synodl.io/kind=ytdl" {
		t.Errorf("labelSelector = %q, want the selector passed through", gotSelector)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
	if len(pods) != 1 || pods[0].Metadata.Name != "p1" || pods[0].Status.Phase != "Running" {
		t.Fatalf("pods = %+v, want one running pod named p1", pods)
	}
}

// The regression for the bug that shipped in 0.16.0-0.16.2.
//
// PodLog sent `Accept: text/plain`, which is the obvious header for a response
// whose body IS plain text — and the API server answers 406 to it:
//
//	only the following media types are accepted: application/json,
//	application/yaml, application/vnd.kubernetes.protobuf
//
// Content negotiation is on the API's own media types, not on what a particular
// subresource happens to return. Every expansion therefore failed with "could
// not read what that link contains", and no playlist ever expanded.
//
// The original test did not catch it because its httptest fake ignored Accept
// entirely — the fake was more permissive than the thing it stood in for. This
// one rejects what the real server rejects.
func TestPodLog_SendsNoUnacceptableAcceptHeader(t *testing.T) {
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reproduce the API server's own content negotiation.
		if a := r.Header.Get("Accept"); a != "" {
			ok := false
			for _, m := range []string{"application/json", "application/yaml", "application/vnd.kubernetes.protobuf", "*/*"} {
				if strings.Contains(a, m) {
					ok = true
				}
			}
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotAcceptable)
				_, _ = w.Write([]byte(`{"message":"only the following media types are accepted: application/json, application/yaml, application/vnd.kubernetes.protobuf"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("worker output\n"))
	}))

	out, err := c.PodLog(context.Background(), "p1", PodLogOptions{Container: "downloader"})
	if err != nil {
		t.Fatalf("PodLog: %v — the API server refuses an Accept it does not serve", err)
	}
	if string(out) != "worker output\n" {
		t.Fatalf("body = %q", out)
	}
}

func TestPodLog_ReadsPlainTextNotJSON(t *testing.T) {
	// The log endpoint returns text/plain. do() decodes JSON and cannot be
	// reused, which is the whole reason PodLog exists separately.
	var gotPath, gotTail, gotContainer string
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTail = r.URL.Query().Get("tailLines")
		gotContainer = r.URL.Query().Get("container")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("line one\nline two\n"))
	}))

	out, err := c.PodLog(context.Background(), "p1", PodLogOptions{Container: "downloader", TailLines: 200})
	if err != nil {
		t.Fatalf("PodLog: %v", err)
	}
	if gotPath != "/api/v1/namespaces/synodl/pods/p1/log" {
		t.Errorf("path = %q, want the log subresource", gotPath)
	}
	if gotTail != "200" || gotContainer != "downloader" {
		t.Errorf("tailLines=%q container=%q, want both passed through", gotTail, gotContainer)
	}
	if string(out) != "line one\nline two\n" {
		t.Errorf("body = %q, want it returned verbatim", out)
	}
}

func TestPodLog_IsBounded(t *testing.T) {
	// FR-013e. A worker that produces far more output than expected must not
	// become a memory problem: the read is capped, and the excess is dropped
	// rather than allocated.
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("x", maxPodLogBytes*3)))
	}))

	out, err := c.PodLog(context.Background(), "p1", PodLogOptions{})
	if err != nil {
		t.Fatalf("PodLog: %v", err)
	}
	if len(out) > maxPodLogBytes {
		t.Fatalf("read %d bytes, want it capped at %d", len(out), maxPodLogBytes)
	}
}

func TestPodLog_MissingPodIsNotFound(t *testing.T) {
	// A pod that has been swept is the ordinary case, not a fault: the caller
	// must be able to tell it apart and simply show no progress.
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"pods \"p1\" not found"}`))
	}))

	_, err := c.PodLog(context.Background(), "p1", PodLogOptions{})
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want it to report as not-found", err)
	}
}

func TestPodLog_ErrorCarriesNoToken(t *testing.T) {
	// Principle III: the ServiceAccount token lives only in a header and must
	// never travel inside an error.
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))

	_, err := c.PodLog(context.Background(), "p1", PodLogOptions{})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "tok") {
		t.Fatalf("error %q leaks the bearer token", err)
	}
	if !IsForbidden(err) {
		t.Fatalf("err = %v, want it to report as forbidden — in practice a missing RBAC verb", err)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"synodl/server/internal/auth"
	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

// The live-update endpoint (spec 1038).
//
// These use an httptest.Recorder driven by a cancelled context, the same shape
// the task-stream tests use: the handler loops until the reader goes away, and
// cancelling stands in for that.

func setYtdlHeartbeat(t *testing.T, beat time.Duration) {
	t.Helper()
	old := ytdlStreamHeartbeat
	ytdlStreamHeartbeat = beat
	t.Cleanup(func() { ytdlStreamHeartbeat = old })
}

// ytdlStreamFor runs the handler until ctx is cancelled and returns the body.
func ytdlStreamFor(t *testing.T, d Deps, lim *streamLimiter, token string, run func()) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodGet, "/v1/ytdl/stream", nil)
	r.Header.Set(sessionHeader, token)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handleYtdlStream(d, lim).ServeHTTP(rec, r.WithContext(ctx))
		close(done)
	}()

	// Let the subscription land before anything is published, or the test would
	// be racing the thing it is measuring.
	deadline := time.After(2 * time.Second)
	for !d.ytdlHub.hasSubscribers() {
		select {
		case <-deadline:
			t.Fatal("the stream never subscribed")
		case <-time.After(time.Millisecond):
		}
	}
	if run != nil {
		run()
	}
	time.Sleep(50 * time.Millisecond) // let the frame be written
	cancel()
	<-done
	return rec
}

func TestYtdlStream_RefusesWithoutASession(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	rec := do(t, h, "GET", "/v1/ytdl/stream", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no session = %d, want 401", rec.Code)
	}
}

// FR-011. Over the cap the client must be told to go back to polling, and the
// refusal must cost nothing — no subscription, no work.
func TestYtdlStream_ShedsOverTheCapAndTellsTheClientToPoll(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})

	full := newStreamLimiter(1)
	if !full.acquire() {
		t.Fatal("could not fill the limiter")
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/ytdl/stream", nil)
	r.Header.Set(sessionHeader, admin[sessionHeader])
	rec := httptest.NewRecorder()
	handleYtdlStream(d, full).ServeHTTP(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("over the cap = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "stream_limit") {
		t.Errorf("body = %q, want a stream_limit code the client can act on", rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After; the client is not told when to try again")
	}
	if d.ytdlHub.hasSubscribers() {
		t.Error("a refused connection took a slot in the hub")
	}
}

// SC-001: nothing changing means nothing sent, beyond what keeps the connection
// open. And `ready` has to arrive first, or the client has nothing to fetch on.
func TestYtdlStream_ReadyThenSilenceThenAHeartbeat(t *testing.T) {
	setYtdlHeartbeat(t, 20*time.Millisecond)
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})

	rec := ytdlStreamFor(t, d, newStreamLimiter(4), admin[sessionHeader], func() {
		time.Sleep(80 * time.Millisecond) // several quiet heartbeats
	})

	body := rec.Body.String()
	if !strings.HasPrefix(body, "event: ready\n") {
		t.Fatalf("stream did not open with ready:\n%s", body)
	}
	if !strings.Contains(body, ":\n\n") {
		t.Errorf("no heartbeat; the proxy would close this connection:\n%s", body)
	}
	if strings.Contains(body, "data: {\"") {
		t.Errorf("a quiet stream sent an update frame:\n%s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering is not off; the proxy would buffer the whole stream")
	}
}

// FR-004: the frame carries the row that changed and nothing else, whatever
// else exists.
func TestYtdlStream_CarriesOnlyTheChangedRow(t *testing.T) {
	setYtdlHeartbeat(t, time.Second)
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	changed := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/aaa")
	// Three more, filling the parallel limit of four: the point is that the
	// frame stays one row wide however many downloads exist alongside it.
	for _, u := range []string{"bbb", "ccc", "ddd"} {
		ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/"+u)
	}

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	d.reconcileYtdlOnce(context.Background()) // seed happens with nobody watching

	rec := ytdlStreamFor(t, d, newStreamLimiter(4), admin[sessionHeader], func() {
		d.reconcileYtdlOnce(context.Background()) // seed again, now that we watch
		jobs.setStatus(t, changed, k8s.JobStatus{Succeeded: 1})
		d.reconcileYtdlOnce(context.Background())
	})

	frames := updateFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("no update frame:\n%s", rec.Body.String())
	}
	last := frames[len(frames)-1]
	rows := len(last.Created) + len(last.Changed) + len(last.Removed)
	if rows != 1 {
		t.Fatalf("frame carried %d rows, want only the one that changed: %+v", rows, last)
	}
	if !strings.Contains(string(last.Changed[0]), changed) {
		t.Fatalf("frame carried the wrong row: %s", last.Changed[0])
	}
}

// A stream authorises once and then runs for hours. A session revoked in the
// meantime must end it — the client's existing 401 handling takes over.
func TestYtdlStream_EndsWhenTheSessionIsRevoked(t *testing.T) {
	setYtdlHeartbeat(t, 20*time.Millisecond)
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/v1/ytdl/stream", nil)
	r.Header.Set(sessionHeader, admin[sessionHeader])
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handleYtdlStream(d, newStreamLimiter(4)).ServeHTTP(rec, r.WithContext(ctx))
		close(done)
	}()
	for !d.ytdlHub.hasSubscribers() {
		time.Sleep(time.Millisecond)
	}

	do(t, h, "DELETE", "/v1/session", "", admin) // sign out elsewhere

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream outlived the session it was opened with")
	}
	if !strings.Contains(rec.Body.String(), "session_expired") {
		t.Errorf("the stream ended without saying why:\n%s", rec.Body.String())
	}
}

// An admin's stream carries EVERYONE's downloads. Continuing to send them to
// somebody whose admin flag has been withdrawn is exactly the disclosure the
// ownership rule exists to prevent.
func TestYtdlStream_EndsWhenAdminIsWithdrawn(t *testing.T) {
	setYtdlHeartbeat(t, 20*time.Millisecond)
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})

	u := userFor(t, st, admin[sessionHeader])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/v1/ytdl/stream", nil)
	r.Header.Set(sessionHeader, admin[sessionHeader])
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handleYtdlStream(d, newStreamLimiter(4)).ServeHTTP(rec, r.WithContext(ctx))
		close(done)
	}()
	for !d.ytdlHub.hasSubscribers() {
		time.Sleep(time.Millisecond)
	}

	if err := st.SetUserAdmin(u.ID, false); err != nil {
		t.Fatalf("withdraw admin: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream kept carrying everyone's downloads to a user who is no longer an admin")
	}
}

// FR-010 / SC-004. The orchestrator is asked by the reconciler alone: no number
// of connections may change how often it is called.
func TestYtdlStream_NeverAsksTheOrchestrator(t *testing.T) {
	setYtdlHeartbeat(t, 10*time.Millisecond)
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})

	before := jobs.calls()
	ytdlStreamFor(t, d, newStreamLimiter(4), admin[sessionHeader], func() {
		time.Sleep(60 * time.Millisecond)
	})
	if after := jobs.calls(); after != before {
		t.Fatalf("the stream made %d orchestrator calls; it must make none", after-before)
	}
}

// updateFrames pulls the update frames out of an SSE body.
func updateFrames(t *testing.T, body string) []ytdlUpdateView {
	t.Helper()
	var out []ytdlUpdateView
	for _, frame := range strings.Split(body, "\n\n") {
		line := strings.TrimSpace(frame)
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var v ytdlUpdateView
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
			continue
		}
		if len(v.Created)+len(v.Changed)+len(v.Removed) > 0 {
			out = append(out, v)
		}
	}
	return out
}

func userFor(t *testing.T, st *store.Store, token string) *store.User {
	t.Helper()
	u, err := st.UserForSession(auth.HashToken(token), time.Now().Unix())
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	return u
}

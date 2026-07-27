package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"synodl/server/internal/syno"
)

// setStreamIntervals shrinks the poll/heartbeat cadence for deterministic tests
// and restores the production values on cleanup.
func setStreamIntervals(t *testing.T, poll, beat time.Duration) {
	t.Helper()
	op, ob := streamPoll, streamHeartbeat
	streamPoll, streamHeartbeat = poll, beat
	t.Cleanup(func() { streamPoll, streamHeartbeat = op, ob })
}

func streamReq(t *testing.T, ctx context.Context, sid string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/v1/tasks/stream", nil)
	if sid != "" {
		r.Header.Set("X-Syno-Sid", sid)
	}
	return r.WithContext(ctx)
}

func TestTasksStreamEmitsHeadersSnapshotAndHeartbeat(t *testing.T) {
	setStreamIntervals(t, 20*time.Millisecond, 5*time.Millisecond)
	fake := &fakeSyno{
		tasks: []syno.Task{{ID: "dbid_1", Name: "live.iso", Status: "downloading"}},
		stats: syno.Stats{DownloadSpeed: 9},
	}
	ctx, cancel := context.WithCancel(context.Background())
	// The stream loops until the client disconnects; cancel stands in for that.
	time.AfterFunc(80*time.Millisecond, cancel)

	rec := httptest.NewRecorder()
	handleTasksStream(Deps{Syno: fake}, newStreamLimiter(4)).ServeHTTP(rec, streamReq(t, ctx, "sid"))

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if xa := rec.Header().Get("X-Accel-Buffering"); xa != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", xa)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, `"live.iso"`) {
		t.Fatalf("body missing snapshot data event:\n%s", body)
	}
	// Identical snapshots are skipped, so the idle connection must emit heartbeat
	// comments to survive the reverse-proxy read timeout.
	if !strings.Contains(body, ":\n\n") {
		t.Errorf("body missing heartbeat comment:\n%s", body)
	}
}

func TestTasksStreamTerminatesOnSessionExpiry(t *testing.T) {
	setStreamIntervals(t, 5*time.Millisecond, time.Hour)
	fake := &fakeSyno{
		tasks:         []syno.Task{{ID: "dbid_1", Name: "live.iso"}},
		failListAfter: 1, // first snapshot ok, then the NAS session dies
		listErr:       &syno.Error{Kind: syno.KindSession, API: "SYNO.DownloadStation.Task"},
	}
	rec := httptest.NewRecorder()
	// No cancel needed: the terminal error must end the stream on its own.
	handleTasksStream(Deps{Syno: fake}, newStreamLimiter(4)).
		ServeHTTP(rec, streamReq(t, context.Background(), "sid"))

	body := rec.Body.String()
	if !strings.Contains(body, "event: error") || !strings.Contains(body, "session_expired") {
		t.Fatalf("expected terminal session_expired event, got:\n%s", body)
	}
}

func TestTasksStreamConnectAuthErrorIs401(t *testing.T) {
	// An invalid (but present) sid fails the very first snapshot: the client must
	// see a clean 401, not a 200 stream that immediately errors.
	fake := &fakeSyno{err: &syno.Error{Kind: syno.KindSession, API: "SYNO.DownloadStation.Task"}}
	rec := httptest.NewRecorder()
	handleTasksStream(Deps{Syno: fake}, newStreamLimiter(4)).
		ServeHTTP(rec, streamReq(t, context.Background(), "sid"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "event-stream") {
		t.Errorf("must not open a stream on connect auth error; Content-Type = %q", ct)
	}
}

func TestTasksStreamRequiresSid(t *testing.T) {
	rec := httptest.NewRecorder()
	handleTasksStream(Deps{Syno: &fakeSyno{}}, newStreamLimiter(4)).
		ServeHTTP(rec, streamReq(t, context.Background(), ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a missing sid", rec.Code)
	}
}

func TestTasksStreamShedsOverCap(t *testing.T) {
	lim := newStreamLimiter(1)
	if !lim.acquire() { // occupy the only slot
		t.Fatal("precondition: first slot should be free")
	}
	rec := httptest.NewRecorder()
	handleTasksStream(Deps{Syno: &fakeSyno{}}, lim).
		ServeHTTP(rec, streamReq(t, context.Background(), "sid"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 over the cap", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("503 must carry a Retry-After header")
	}
}

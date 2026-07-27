package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// Stream cadence. Vars (not consts) so tests can shrink them. The poll interval
// is a fixed server constant — never client-tunable — so the endpoint can't be
// coaxed into hammering the NAS. The heartbeat comment keeps the connection
// alive through the operator's Synology reverse-proxy 60s read timeout even when
// no snapshot changes.
var (
	streamPoll      = 1 * time.Second
	streamHeartbeat = 15 * time.Second
)

// snapshotFunc fetches one task-list + stats snapshot from the NAS. The two
// stateful/stateless variants differ only in how they obtain the sid, so they
// share the SSE loop below by passing a closure.
type snapshotFunc func(ctx context.Context) ([]syno.Task, syno.Stats, error)

// handleTasksStream is the stateless (dev/e2e) live stream: the client carries
// the NAS sid in X-Syno-Sid, exactly like GET /v1/tasks.
func handleTasksStream(d Deps, lim *streamLimiter) http.Handler {
	return requireSid(func(w http.ResponseWriter, r *http.Request, sid string) {
		snap := func(ctx context.Context) ([]syno.Task, syno.Stats, error) {
			tasks, err := d.Syno.ListTasks(ctx, sid)
			if err != nil {
				return nil, syno.Stats{}, err
			}
			stats, _ := d.Syno.Stats(ctx, sid) // stats failure degrades to zeros
			return tasks, stats, nil
		}
		runTaskStream(w, r, lim, snap, writeSynoError)
	})
}

// handleTasksStreamStateful is the production live stream: the client carries a
// SynoDL session token and NAS calls go through the shared stored connection.
func handleTasksStreamStateful(d Deps, lim *streamLimiter) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		snap := func(ctx context.Context) ([]syno.Task, syno.Stats, error) {
			var tasks []syno.Task
			var stats syno.Stats
			err := d.NAS.Do(ctx, func(c syno.Client, sid string) error {
				t, e := c.ListTasks(ctx, sid)
				if e != nil {
					return e
				}
				tasks = t
				stats, _ = c.Stats(ctx, sid)
				return nil
			})
			if err != nil {
				return nil, syno.Stats{}, err
			}
			return tasks, stats, nil
		}
		runTaskStream(w, r, lim, snap, writeNASError)
	})
}

// runTaskStream is the shared SSE loop. It never persists anything, holds the
// session only for the life of the connection, and logs nothing sensitive. On a
// mid-stream session expiry it emits a terminal error event so the client
// returns to login; on any other error it just ends the stream so the client
// falls back to ordinary polling.
func runTaskStream(w http.ResponseWriter, r *http.Request, lim *streamLimiter, snap snapshotFunc, mapErr func(http.ResponseWriter, error)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "server")
		return
	}

	// Bound concurrent streams before touching the NAS: an over-cap connection
	// must cost nothing upstream. Shed with 503 + Retry-After; the client polls.
	if !lim.acquire() {
		w.Header().Set("Retry-After", "5")
		httpx.Error(w, http.StatusServiceUnavailable, "stream_limit")
		return
	}
	defer lim.release()

	ctx := r.Context()

	// First snapshot before committing to 200, so an invalid/expired session
	// yields a clean 401 (existing mapping) instead of a 200 stream that dies on
	// its first line.
	tasks, stats, err := snap(ctx)
	if err != nil {
		mapErr(w, err)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // tell nginx/Synology proxy not to buffer
	w.WriteHeader(http.StatusOK)

	last := marshalSnapshot(tasks, stats)
	writeSSEData(w, last)
	flusher.Flush()

	poll := time.NewTicker(streamPoll)
	defer poll.Stop()
	beat := time.NewTicker(streamHeartbeat)
	defer beat.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected or server shutting down — stop promptly.
			return
		case <-poll.C:
			tasks, stats, err := snap(ctx)
			if err != nil {
				if isSessionExpiry(err) {
					_, _ = io.WriteString(w, "event: error\ndata: {\"error\":\"session_expired\"}\n\n")
					flusher.Flush()
				}
				return
			}
			payload := marshalSnapshot(tasks, stats)
			if bytes.Equal(payload, last) {
				continue // nothing changed — let the heartbeat carry the connection
			}
			last = payload
			writeSSEData(w, payload)
			flusher.Flush()
			beat.Reset(streamHeartbeat) // a snapshot counts as keep-alive traffic
		case <-beat.C:
			_, _ = io.WriteString(w, ":\n\n") // SSE comment; clients ignore it
			flusher.Flush()
		}
	}
}

// marshalSnapshot renders the same {tasks, stats} shape as GET /v1/tasks so the
// stream and the poll are byte-compatible on the client.
func marshalSnapshot(tasks []syno.Task, stats syno.Stats) []byte {
	if tasks == nil {
		tasks = []syno.Task{}
	}
	b, _ := json.Marshal(map[string]any{"tasks": tasks, "stats": stats})
	return b
}

func writeSSEData(w io.Writer, payload []byte) {
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
}

// isSessionExpiry reports whether a NAS error means the session is gone and the
// client must return to login (vs a transient error where it falls back to
// polling). Mirrors the 401 group in writeSynoError.
func isSessionExpiry(err error) bool {
	se := syno.AsError(err)
	if se == nil {
		return false
	}
	switch se.Kind {
	case syno.KindSession, syno.KindCredentials, syno.KindOTPRequired,
		syno.KindOTPInvalid, syno.KindPasswordExpired:
		return true
	default:
		return false
	}
}

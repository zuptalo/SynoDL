package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"synodl/server/internal/auth"
	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// GET /v1/ytdl/stream — YouTube downloads, live (spec 1038).
//
// The transport is the one spec 0006 settled for NAS tasks and every reason
// still holds: SSE over a plain response, a heartbeat comment through the
// operator's reverse proxy, X-Accel-Buffering off, a bound taken before any
// work, and a 503 that tells the client to go back to polling rather than an
// error it has to interpret.
//
// What is DIFFERENT from the task stream is where the content comes from. That
// one polls the NAS on its own ticker and sends a whole snapshot. This one sends
// nothing on its own initiative at all: it forwards what the reconciler
// announced, which is what makes ten viewers cost what one does (SC-002a) and
// what keeps the orchestrator being asked exactly as often as before (FR-010).

// ytdlStreamHeartbeat is how long a quiet stream waits before saying something.
//
// Fifteen seconds, the same as the task stream, for the same reason: the
// operator's Synology reverse proxy closes a read after sixty.
var ytdlStreamHeartbeat = 15 * time.Second

// ytdlUpdateView is one frame.
//
// Three lists rather than one, because a client does three different things
// with them: insert, merge, drop. Collapsing them would put the distinction in
// a flag on each row and make "did I already have this?" the client's problem
// (FR-004a).
type ytdlUpdateView struct {
	Created []json.RawMessage `json:"created,omitempty"`
	Changed []json.RawMessage `json:"changed,omitempty"`
	Removed []string          `json:"removed,omitempty"`
}

// handleYtdlStream subscribes the caller and forwards what they may see.
func handleYtdlStream(d Deps, lim *streamLimiter) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}

		// Nothing to stream where the feature does not exist. A 503 rather than
		// a 404 so it reads as "not on this deployment", which is what every
		// other ytdl endpoint says outside a cluster.
		if !d.ytdlAvailable() || d.ytdlHub == nil {
			w.Header().Set("Retry-After", "60")
			httpx.Error(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		// Bound before subscribing: an over-cap connection must cost nothing,
		// not even a slot in the hub. The client polls instead (FR-011).
		if !lim.acquire() {
			w.Header().Set("Retry-After", "5")
			httpx.Error(w, http.StatusServiceUnavailable, "stream_limit")
			return
		}
		defer lim.release()

		ch, unsubscribe := d.ytdlHub.subscribe(u.ID, u.IsAdmin)
		defer unsubscribe()

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no") // nginx / Synology proxy: do not buffer
		w.WriteHeader(http.StatusOK)

		// `ready` is what closes the connect gap. The client fetches its list ON
		// this rather than before connecting, so a change that lands while the
		// connection is being established is carried by the fetch or by the
		// stream instead of falling between them — which is also why the stream
		// itself never has to replay anything (FR-004b).
		_, _ = io.WriteString(w, "event: ready\ndata: {}\n\n")
		flusher.Flush()

		ctx := r.Context()
		token := r.Header.Get(sessionHeader)
		beat := time.NewTicker(ytdlStreamHeartbeat)
		defer beat.Stop()

		for {
			select {
			case <-ctx.Done():
				// The reader went away, or the server is shutting down.
				return
			case batch, open := <-ch:
				if !open {
					// The hub dropped us for falling behind (FR-012b). Ending the
					// stream is how the client finds out: it reconnects and
					// recovers by re-reading what it is showing.
					return
				}
				payload, ok := ytdlFrameFor(batch, u.IsAdmin)
				if !ok {
					continue
				}
				writeSSEData(w, payload)
				flusher.Flush()
				beat.Reset(ytdlStreamHeartbeat) // real traffic is keep-alive too
			case <-beat.C:
				// A stream outlives the request that authorised it, so the
				// heartbeat is also when it re-proves it may still be sent what
				// it is being sent. Once per fifteen seconds per connection is
				// nothing next to being wrong about it for hours.
				if !ytdlStreamStillAllowed(d, token, u) {
					_, _ = io.WriteString(w, "event: error\ndata: {\"error\":\"session_expired\"}\n\n")
					flusher.Flush()
					return
				}
				_, _ = io.WriteString(w, ":\n\n") // SSE comment; clients ignore it
				flusher.Flush()
			}
		}
	})
}

// ytdlFrameFor renders one batch for one reader.
//
// Returns false for a batch that says nothing to this reader, so a quiet stream
// stays genuinely quiet (SC-001) rather than sending empty frames.
func ytdlFrameFor(batch []ytdlChange, isAdmin bool) ([]byte, bool) {
	var out ytdlUpdateView
	for _, c := range batch {
		switch {
		case c.Removed:
			out.Removed = append(out.Removed, c.RequestID)
		case c.Created:
			out.Created = append(out.Created, c.payload(isAdmin))
		default:
			out.Changed = append(out.Changed, c.payload(isAdmin))
		}
	}
	if len(out.Created) == 0 && len(out.Changed) == 0 && len(out.Removed) == 0 {
		return nil, false
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return b, true
}

// ytdlStreamStillAllowed re-resolves the session behind a live connection.
//
// A poll re-authenticates on every request; a stream authenticates once and may
// then run for hours. Both halves matter: a session that has been revoked, and
// an admin flag that has been withdrawn — the second because an admin's stream
// carries everyone's downloads, and continuing to send them to somebody who is
// no longer an admin is the disclosure the ownership rule exists to prevent.
func ytdlStreamStillAllowed(d Deps, token string, u *store.User) bool {
	if token == "" || d.Store == nil {
		return false
	}
	now, err := d.Store.UserForSession(auth.HashToken(token), time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		// Could not tell. Keep the connection: a database hiccup is not evidence
		// that a session ended, and dropping every stream on one would be its own
		// outage. The next heartbeat asks again.
		return true
	}
	return now.ID == u.ID && now.IsAdmin == u.IsAdmin
}

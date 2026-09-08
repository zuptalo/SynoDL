package api

import (
	"sync"
	"time"

	"synodl/server/internal/ytdl"
)

// Where a running download's progress lives (spec 0013).
//
// In MEMORY, and that is a constitutional decision rather than a performance
// one. Constitution v2.2.0 permits a durable record of the request and its
// finished outcome, and still forbids mirroring in-flight worker state. A
// percentage is in-flight state: it belongs to the worker, it changes every
// second, and it is meaningless once the worker is gone. So it is held as a
// CACHE OF A READING — lost on restart, with no consequence beyond a bar that
// starts reporting again on the next cycle.
//
// It is also why reading a worker's output is bounded in frequency (FR-013f):
// the reconciler fills this on its own clock, and every request handler just
// reads what is already here. A hundred people watching the page produce zero
// extra calls to the orchestrator.

// progressTTL is how long a reading stays usable.
//
// A reading older than this means the reconciler has stopped hearing from the
// worker — the pod was swept, or the log went unreadable — and FR-013 says to
// show NO percentage rather than a stale one that looks like a stall.
const progressTTL = 90 * time.Second

type progressEntry struct {
	tracker ytdl.ProgressTracker
	value   float64
	at      time.Time
}

// progressCache holds the latest reading per running download.
type progressCache struct {
	mu sync.Mutex
	m  map[string]*progressEntry
}

func newProgressCache() *progressCache {
	return &progressCache{m: map[string]*progressEntry{}}
}

// Observe records a fraction for a download and returns the value to show.
//
// Clamped monotonically HERE rather than in the client, because several viewers
// read the same held value — so the value itself has to already be correct
// (FR-012).
func (c *progressCache) Observe(requestID string, f float64) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[requestID]
	if !ok {
		e = &progressEntry{}
		c.m[requestID] = e
	}
	e.value = e.tracker.Observe(f)
	e.at = time.Now()
	return e.value
}

// Get returns the held reading, or false when there is none or it is stale.
func (c *progressCache) Get(requestID string) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[requestID]
	if !ok || time.Since(e.at) > progressTTL {
		return 0, false
	}
	return e.value, true
}

// Forget drops a download's reading once it is finished — a completed download
// has no progress, and keeping the entry would leak one map key per download
// for the life of the process.
func (c *progressCache) Forget(requestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, requestID)
}

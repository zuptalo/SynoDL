package api

import "sync/atomic"

// streamLimiter is a global cap on concurrent SSE task streams. It mirrors the
// spirit of the per-IP login rate limiter: the live-update endpoint holds a
// long-lived connection that polls the NAS, so an unbounded number of them
// would let clients hammer the NAS indefinitely. Excess connections are shed
// (503 + Retry-After) and the client falls back to ordinary polling.
type streamLimiter struct {
	max int64
	n   atomic.Int64
}

// newStreamLimiter builds a limiter that admits at most max concurrent streams.
// A non-positive max is floored to one so the bound can never be disabled.
func newStreamLimiter(max int) *streamLimiter {
	if max < 1 {
		max = 1
	}
	return &streamLimiter{max: int64(max)}
}

// acquire reserves a slot, returning false when the cap is already reached. A
// rejected caller does not hold a slot and must not call release.
func (l *streamLimiter) acquire() bool {
	if l.n.Add(1) > l.max {
		l.n.Add(-1)
		return false
	}
	return true
}

// release returns a slot taken by a successful acquire.
func (l *streamLimiter) release() { l.n.Add(-1) }

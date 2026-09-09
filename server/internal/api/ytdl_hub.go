package api

import (
	"encoding/json"
	"sync"

	"synodl/server/internal/store"
)

// Fanning a change out to whoever is watching (spec 1038).
//
// The shape of this file follows from one requirement and one measurement.
//
// The requirement is FR-012: a change is announced by whatever made it, at the
// moment it is made. The alternative — each connection re-reading on its own
// clock — is polling moved server-side, and it multiplies database reads by the
// number of watchers.
//
// The measurement is SC-002a: ten viewers must cost what one does. That is why a
// change arrives here ALREADY RENDERED, and rendered exactly twice — once as an
// admin sees it, once as its owner does. Rendering per subscriber would put the
// cost back where the requirement took it from.
//
// Nothing here is durable and nothing here is authoritative. A subscriber that
// misses an update recovers by re-reading what it is showing (FR-004b), which is
// what lets every send be non-blocking and every slow reader be dropped.

// ytdlChange is one download's change, ready to send.
//
// Both renderings are carried because `submittedBy` is admin-only: the same
// change is a different object depending on who is reading it, and deciding that
// per subscriber would mean marshalling per subscriber.
type ytdlChange struct {
	// OwnerID is who the download belongs to, or nil for a record whose account
	// was deleted (spec 0013, FR-006d) — visible to admins only.
	OwnerID *int64
	// Created distinguishes a download the watcher has never published from one
	// it has. A client inserts the first and merges the second; see FR-004a.
	Created bool
	// Removed means the download is gone (dismissed). Admin and Plain are unset.
	Removed bool
	// Item is true when this download hangs behind a group. Such a row only ever
	// concerns a client with that group's sheet open.
	Item bool

	RequestID string
	Admin     json.RawMessage
	Plain     json.RawMessage
}

// visibleTo reports whether a reader may be told about this change.
//
// Deliberately the same rule ytdlVisibleTo states for the list, expressed
// against the same two inputs, so a stream cannot end up showing something a
// poll would not.
func (c ytdlChange) visibleTo(userID int64, isAdmin bool) bool {
	if isAdmin {
		return true
	}
	return c.OwnerID != nil && *c.OwnerID == userID
}

// payload picks the rendering this reader is entitled to.
func (c ytdlChange) payload(isAdmin bool) json.RawMessage {
	if isAdmin {
		return c.Admin
	}
	return c.Plain
}

// ytdlSubscriber is one live connection.
type ytdlSubscriber struct {
	userID  int64
	isAdmin bool
	// ch is buffered. A reader that falls this far behind is dropped rather than
	// allowed to grow a backlog (FR-012b) — see publish.
	ch     chan []ytdlChange
	closed bool
}

// ytdlSubscriberBuffer is how many undelivered batches a connection may hold.
//
// Small on purpose. A batch is at most one reconcile cycle's worth of change, so
// eight of them is roughly twenty-four seconds of falling behind — long past the
// point where reconnecting and re-reading is the better answer.
const ytdlSubscriberBuffer = 8

// ytdlHub holds the subscribers and fans changes out to them.
type ytdlHub struct {
	mu   sync.Mutex
	subs map[*ytdlSubscriber]struct{}
}

func newYtdlHub() *ytdlHub {
	return &ytdlHub{subs: map[*ytdlSubscriber]struct{}{}}
}

// subscribe registers a reader and returns its channel plus a cancel.
//
// The cancel is idempotent and MUST be deferred by the caller: a connection that
// went away without unsubscribing would keep a slot and a buffer for the life of
// the process.
func (h *ytdlHub) subscribe(userID int64, isAdmin bool) (<-chan []ytdlChange, func()) {
	s := &ytdlSubscriber{
		userID:  userID,
		isAdmin: isAdmin,
		ch:      make(chan []ytdlChange, ytdlSubscriberBuffer),
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()

	return s.ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.drop(s)
	}
}

// drop removes a subscriber and closes its channel. Caller holds the lock.
//
// Closing is what tells the connection to end — a dropped reader must find out,
// or it would sit on a stream that has stopped carrying anything.
func (h *ytdlHub) drop(s *ytdlSubscriber) {
	if _, ok := h.subs[s]; !ok {
		return
	}
	delete(h.subs, s)
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// hasSubscribers reports whether anyone is watching.
//
// This is what lets the whole diff be skipped when nobody is (FR-010). A server
// nobody is looking at does exactly what it did before this feature existed —
// which matters, because the reconciler runs every three seconds for the life of
// the process.
func (h *ytdlHub) hasSubscribers() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs) > 0
}

// publish fans a batch out to every subscriber entitled to some of it.
//
// Two properties are load-bearing:
//
// It NEVER BLOCKS (FR-012a). This is called from the reconciler, which also
// admits queued work and captures outcomes that exist nowhere else once a worker
// is swept. A reader that stopped reading must not be able to hold that up.
//
// A reader that cannot keep up is DROPPED (FR-012b), not queued. Its connection
// ends, it reconnects, and it recovers by re-reading what it is showing — which
// is exactly the recovery FR-004b already requires for a dropped connection, so
// this costs no additional mechanism.
func (h *ytdlHub) publish(changes []ytdlChange) {
	if h == nil || len(changes) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	for s := range h.subs {
		mine := make([]ytdlChange, 0, len(changes))
		for _, c := range changes {
			if c.visibleTo(s.userID, s.isAdmin) {
				mine = append(mine, c)
			}
		}
		if len(mine) == 0 {
			continue
		}
		select {
		case s.ch <- mine:
		default:
			h.drop(s)
		}
	}
}

// ytdlOwnerOf is the owner id a change carries, stated once so the watch step
// and the ownership rule cannot disagree about what "unowned" means.
func ytdlOwnerOf(rec store.YtdlDownload) *int64 { return rec.UserID }

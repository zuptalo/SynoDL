package api

import (
	"bytes"
	"encoding/json"
	"errors"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

// Working out what changed, once, for everybody (spec 1038).
//
// This runs at the END of a reconcile cycle, and where it runs is the whole
// design rather than a detail of where the code landed.
//
// A YouTube download's shown state has four sources. Two of them — whether its
// worker is running, and how far along it is — are never written anywhere: they
// are read from the orchestrator's job list and from an in-memory cache, both of
// which only exist inside a cycle. A stream that re-read the STORE on its own
// clock would therefore still not see a download start downloading until the
// reconciler noticed. It would be polling one thing against state owned by
// another.
//
// So the diff happens where the answers already are, holding the same job list
// the rest of the cycle used.
//
// It compares the PROJECTED VIEW, not the stored row. That is what makes a
// change the orchestrator caused and a change the store recorded look identical
// here, and it is what stops this drifting from what the list endpoint returns:
// it calls the same projection.
//
// Nothing here is stored. The fingerprint map is in memory and is rebuilt from a
// cycle, exactly as the progress cache is — Principle III is untouched, because
// a fingerprint is not a mirror of job state, it is a record of what was already
// said out loud.

// ytdlSeenRow is what was last said about one download.
//
// The owner is remembered alongside the fingerprint because a REMOVAL has no
// record left to read it from: by the time the watcher notices a dismissal the
// row is gone, and "who was allowed to know about this?" is a question only the
// map can still answer. Without it a removal could only be sent to admins, and
// the person who dismissed it would be the one left with a lingering row.
type ytdlSeenRow struct {
	fp    []byte
	owner *int64
	item  bool
}

// ytdlFingerprints remembers what was last published about each download.
//
// Keyed by request id, fingerprinted on the marshalled owner-view. The admin
// view is not fingerprinted separately: the two differ only by `submittedBy`,
// which changes when the account is renamed and not otherwise, and a rename is
// not something a live update needs to chase.
type ytdlFingerprints struct {
	seen map[string]ytdlSeenRow
	// spoken is false until this process has published anything.
	//
	// The very first publish carries the whole unfinished set, because there was
	// nothing to compare it against. Those rows go out as CHANGED rather than
	// created: a client merges a change it recognises and ignores one it does
	// not, whereas `created` means "put this at the top of the list" — and a
	// restart is not a reason to reorder somebody's list.
	spoken bool
}

func newYtdlFingerprints() *ytdlFingerprints {
	return &ytdlFingerprints{seen: map[string]ytdlSeenRow{}}
}

// watchYtdl publishes what changed this cycle.
//
// Returns immediately unless somebody is watching. That is not an optimisation
// so much as a promise: a server nobody is looking at does exactly what it did
// before this feature existed — no extra query, no projection, no memory held —
// and the reconciler runs every three seconds for the life of the process.
func (d Deps) watchYtdl(live map[string]k8s.Job) {
	if d.Store == nil || d.ytdlHub == nil || d.ytdlSeen == nil {
		return
	}
	if !d.ytdlHub.hasSubscribers() {
		// Nobody watching: no query, no projection, nothing held. A server nobody
		// is looking at does exactly what it did before this feature existed.
		//
		// What was already remembered is deliberately KEPT rather than cleared.
		// An earlier version cleared it and re-seeded on the first cycle after
		// somebody connected — publishing nothing on that cycle — which silently
		// swallowed everything that happened between the client fetching its list
		// and that seed. It showed up as a download whose progress arrived inside
		// exactly that window and was then never mentioned again, because from the
		// next cycle on it matched. Keeping the map means the first cycle back
		// reports what changed while nobody was looking, which is both correct and
		// smaller than a re-seed would have been.
		return
	}

	records, err := d.Store.ListYtdlUnfinished()
	if err != nil {
		// Not fatal and not logged per-cycle, like everything else in this loop.
		// A watcher simply hears nothing this time round.
		return
	}

	changes := make([]ytdlChange, 0, 8)
	current := make(map[string]ytdlSeenRow, len(records))

	for _, rec := range records {
		c, fp, ok := d.ytdlChangeFor(rec, live)
		if !ok {
			continue
		}
		current[rec.RequestID] = ytdlSeenRow{fp: fp, owner: c.OwnerID, item: c.Item}
		prev, known := d.ytdlSeen.seen[rec.RequestID]
		if known && bytes.Equal(prev.fp, fp) {
			continue // nothing to say about this one
		}
		// `created` is what makes a client INSERT a row it does not already hold.
		// On this process's first publish nothing is known, so every row would
		// look new — see ytdlFingerprints.spoken.
		c.Created = !known && d.ytdlSeen.spoken
		changes = append(changes, c)
	}

	// Anything that was in the watch set and no longer is has either FINISHED or
	// been dismissed, and those are opposite things to tell a client. Resolved
	// one at a time, which is bounded by how many left the set this cycle rather
	// than by how much history exists.
	for id, was := range d.ytdlSeen.seen {
		if _, still := current[id]; still {
			continue
		}
		rec, err := d.Store.GetYtdlDownload(id)
		if errors.Is(err, store.ErrNotFound) {
			// Dismissed. A watching client must be told, or the row lingers on a
			// screen for something that no longer exists (FR-013). The owner
			// comes from what was remembered, because there is no record left to
			// ask — and sending it to admins only would leave the very person who
			// dismissed it looking at the row they got rid of.
			changes = append(changes, ytdlChange{
				RequestID: id, Removed: true, OwnerID: was.owner, Item: was.item,
			})
			continue
		}
		if err != nil {
			// Could not tell which it was. Keep the fingerprint so the next cycle
			// asks again rather than guessing at "removed", which is the one
			// answer that cannot be taken back.
			current[id] = was
			continue
		}
		// It reached a terminal state. Say so once, then let it go: a final
		// download cannot change again, so keeping it in the map would only cost
		// a lookup per cycle forever.
		if c, _, ok := d.ytdlChangeFor(rec, live); ok {
			changes = append(changes, c)
		}
	}

	d.ytdlSeen.seen = current
	d.ytdlSeen.spoken = true
	d.ytdlHub.publish(changes)
}

// ytdlChangeFor projects one record exactly as the list endpoint would, and
// renders it for both kinds of reader.
//
// Rendered TWICE, never once per subscriber: `submittedBy` is admin-only, so the
// same change is a different object depending on who reads it, and deciding that
// per connection is the multiplication SC-002a forbids.
//
// A removal cannot be produced here — it has no record to project — so `ok` is
// false when the row cannot be rendered at all.
func (d Deps) ytdlChangeFor(rec store.YtdlDownload, live map[string]k8s.Job) (ytdlChange, []byte, bool) {
	state, reason := liveStateOf(rec, live)

	// Deliberately NOT captureTerminal: writing an outcome is the reconciler's
	// job and it has already run this cycle. A watcher observes; it must not be
	// able to change what it is watching.
	plain, err := json.Marshal(d.ytdlViewOf(rec, state, reason, &store.User{ID: ownerIDOrZero(rec), IsAdmin: false}))
	if err != nil {
		return ytdlChange{}, nil, false
	}
	admin, err := json.Marshal(d.ytdlViewOf(rec, state, reason, &store.User{IsAdmin: true}))
	if err != nil {
		return ytdlChange{}, nil, false
	}

	return ytdlChange{
		OwnerID:   ytdlOwnerOf(rec),
		Item:      rec.ParentID != "",
		RequestID: rec.RequestID,
		Admin:     admin,
		Plain:     plain,
	}, plain, true
}

// ownerIDOrZero is the owner's id, or zero for an unowned record.
//
// Only ever used to build the non-admin projection, which reads the id for
// nothing but the admin check it is about to fail. Stated rather than inlined so
// it is obvious no ownership decision is being made here — that lives in
// ytdlChange.visibleTo, against the record's real owner.
func ownerIDOrZero(rec store.YtdlDownload) int64 {
	if rec.UserID == nil {
		return 0
	}
	return *rec.UserID
}

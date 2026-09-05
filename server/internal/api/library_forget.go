package api

import (
	"context"
	"time"

	"strings"

	"synodl/server/internal/library"
)

// Forgetting content that has left the NAS (spec 1029).
//
// SynoDL remembers what it sent to each title folder, and that record is the only
// thing that knows WHICH version a user downloaded — a library renamed for a
// media server carries no release information in its file names, so it cannot be
// recovered afterwards (spec 0010, spec 2015). Deleting one is therefore
// irreversible, which shapes everything below: the reconciliation is slow,
// refuses to act on anything it is unsure about, and treats "I could not look"
// as entirely different from "I looked and it is empty".

// forgetGrace is how long a title folder must be seen gone, continuously, before
// the record of what was sent there is deleted.
//
// Long, deliberately. Several ordinary states look exactly like "empty" — a
// download in flight, an archive not yet extracted, a folder a media server is
// moving — and the cost of waiting is nothing a user can see, because a folder
// with no video already reports as not-owned from the live reading. The cost of
// being hasty is data nobody can get back.
const forgetGrace = 24 * time.Hour

// folderState is what the current reading says about one recorded destination.
type folderState int

const (
	// folderUnknown: we could not determine anything. Nothing may be concluded.
	folderUnknown folderState = iota
	// folderPresent: a matching folder exists and holds video.
	folderPresent
	// folderGone: no matching folder, or one that holds no video.
	folderGone
)

// forgetRemovedContent reconciles every recorded download against the reading the
// scan has just produced, marking what is gone and deleting what has been gone
// long enough.
//
// It performs no NAS reads of its own. Everything it needs is either in the name
// index the scan just built or in the stored folder readings, so a library of any
// size costs one query and some comparisons (FR-005, SC-005).
func (d Deps) forgetRemovedContent(ctx context.Context, ix *library.Index) {
	if d.Store == nil || ix == nil || ix.IsEmpty() {
		// An empty index means either nothing is configured or nothing could be
		// read. Neither is evidence that anything was removed (FR-010).
		return
	}
	records, err := d.Store.SourceDownloads()
	if err != nil {
		return
	}
	pending := d.pendingDestinations()
	now := time.Now()

	for dest, rec := range records {
		if ctx.Err() != nil {
			return
		}
		// A folder with an unfinished task is empty for a reason (FR-009). Paused
		// counts: it resumes, and its folder fills then.
		if pending[dest] {
			if !rec.MissingSince.IsZero() {
				_ = d.Store.ClearSourceDownloadMissing(dest)
			}
			continue
		}
		switch d.stateOfRecordedFolder(ix, dest) {
		case folderPresent:
			if !rec.MissingSince.IsZero() {
				// It came back. Forget that it was ever gone (FR-007).
				_ = d.Store.ClearSourceDownloadMissing(dest)
			}
		case folderGone:
			if rec.MissingSince.IsZero() {
				// First sighting. Mark it and do nothing else (FR-005).
				_ = d.Store.MarkSourceDownloadMissing(dest, now)
				continue
			}
			if now.Sub(rec.MissingSince) >= forgetGrace {
				// Gone, and gone for long enough (FR-006). Only this record — the
				// per-user history is an append-only statistics and quota log and is
				// deliberately untouched (FR-011).
				_ = d.Store.DeleteSourceDownload(dest)
			}
		case folderUnknown:
			// We could not look. Say nothing (FR-008).
		}
	}
}

// stateOfRecordedFolder decides what the reading says about one recorded
// destination.
//
// The destination is the folder SynoDL SENT to, which is routinely not the folder
// the content lives in now — a media server renames it, most often by appending
// the release year. So the folder is found the way the index finds it, by name
// comparison rather than by exact text (FR-004). Getting this wrong would report
// every renamed folder as absent, which on a managed library is all of them.
func (d Deps) stateOfRecordedFolder(ix *library.Index, dest string) folderState {
	entry, matched := lookupRecordedFolder(ix, dest)
	if !matched {
		// Nothing under any configured parent could be this title. The index was
		// built from a successful listing, so this is a real answer (FR-002).
		return folderGone
	}
	rec, found := d.storedFolderEvidence(entry.Path)
	if !found {
		// The folder exists but has never been read. Not evidence of anything;
		// the scan will get to it (FR-008).
		return folderUnknown
	}
	if rec.hasVideo {
		return folderPresent
	}
	return folderGone // read, and holds no video (FR-003)
}

// lookupRecordedFolder finds the NAS folder a recorded destination refers to.
//
// A series records one destination per season folder, so the season segment is
// stripped first and the TITLE folder is what is looked up: seasons come and go
// under a title that is still there.
func lookupRecordedFolder(ix *library.Index, dest string) (library.Entry, bool) {
	parent, _, _ := titleFolderIdentity(dest)
	if parent == "" {
		return library.Entry{}, false
	}
	name := titleFolderName(dest)
	if name == "" {
		return library.Entry{}, false
	}
	// Which parent the record sits under says whether it is a film or a series,
	// which is how the index disambiguates two titles that share a name.
	for _, kind := range []library.MediaKind{library.MediaMovie, library.MediaTV} {
		if entry, ok := ix.Lookup(name, kind); ok && sameParent(entry.Path, parent) {
			return entry, true
		}
	}
	return library.Entry{}, false
}

// titleFolderName is the name of the TITLE folder a destination refers to, with
// any trailing season segment removed.
func titleFolderName(dest string) string {
	segs := strings.Split(strings.Trim(strings.TrimSpace(dest), "/"), "/")
	if len(segs) > 2 {
		if _, ok := library.SeasonOfFolder(segs[len(segs)-1]); ok {
			segs = segs[:len(segs)-1]
		}
	}
	if len(segs) < 2 {
		return ""
	}
	return segs[len(segs)-1]
}

// sameParent reports whether an index entry sits under the parent a record was
// written against. Without it a film and a series of the same name would answer
// for each other.
func sameParent(entryPath, parent string) bool {
	i := strings.LastIndex(entryPath, "/")
	if i < 0 {
		return false
	}
	return entryPath[:i] == parent
}

// pendingDestinations is the watcher's view of which folders have a task that has
// not finished.
func (d Deps) pendingDestinations() map[string]bool {
	if d.PendingDests == nil {
		return nil
	}
	return d.PendingDests()
}

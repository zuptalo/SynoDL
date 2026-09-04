package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/library"
	"synodl/server/internal/source"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// libraryTTL bounds how stale the "what do we already have?" reading may be.
//
// The trade-off, settled in the spec (FR-010, FR-010a): re-reading per catalog
// page would put a NAS round-trip in front of every infinite-scroll fetch, and
// reading once per process would hide anything added to the NAS by other means
// for the life of the container. Five minutes bounds the out-of-band case, while
// explicit invalidation on send covers the case a user will actually notice —
// their own download appearing immediately.
const libraryTTL = 5 * time.Minute

// libraryCache holds the current snapshot. It lives behind a pointer on Deps
// because Deps is copied by value into every handler closure — a value field
// here would give each handler its own private cache and defeat the point.
//
// Deliberately in memory and nowhere else: the NAS is the source of truth
// (Principle III), so a rebuild-on-demand cache needs no migration, cannot drift
// across a restart, and never becomes a durable copy of the user's library.
type libraryCache struct {
	mu      sync.Mutex
	index   *library.Index
	builtAt time.Time

	// evidence is layer two: what a SPECIFIC title folder was found to contain.
	// Layer one (index) only says a folder with a matching name exists, which
	// 0.3.0 mistook for proof the content was there. Populated lazily and only for
	// folders backing a title actually being shown (FR-010b), so a page of titles
	// that match nothing costs no NAS read at all.
	evidence map[string]evidenceRec
}

// evidenceRec is one folder's answer, with its own expiry so a folder checked
// long ago is re-read even while the name index is still warm.
type evidenceRec struct {
	hasVideo  bool
	checkedAt time.Time
}

// maxSeasonScan bounds how many subfolders are opened looking for video. A title
// folder has a handful of seasons; anything beyond this is a mis-configured parent
// pointed at something enormous, and walking it would hold up a catalog response.
const maxSeasonScan = 24

// invalidateLibrary drops the snapshot so the next lookup rebuilds it.
//
// Called after a successful send (FR-008), so a title the user just downloaded
// is marked at once rather than after the TTL, and whenever a source's parent
// folders change or a source is added, disabled, or removed (FR-008a), so a
// snapshot can never answer for folders that are no longer configured.
func (d Deps) invalidateLibrary() {
	if d.lib == nil {
		return
	}
	d.lib.mu.Lock()
	// Both layers, together. Keeping folder evidence across an invalidation would
	// answer for a folder the new configuration may no longer include (FR-008a),
	// and would hide the content of a title just sent (FR-008).
	d.lib.index, d.lib.builtAt, d.lib.evidence = nil, time.Time{}, nil
	d.lib.mu.Unlock()
}

// libraryIndex returns a snapshot of the configured parent folders, rebuilding
// it when there is none or it has aged past the TTL.
//
// It never returns an error, and that is the point: every failure — no source
// configured, an unreachable NAS, a parent folder that is missing or that the
// account cannot read — collapses into an empty index that matches nothing
// (FR-009). The caller shows no marker, and the user's browsing is untouched.
func (d Deps) libraryIndex(ctx context.Context) *library.Index {
	if d.lib == nil || d.Store == nil || d.NAS == nil {
		return library.Empty(time.Now())
	}
	d.lib.mu.Lock()
	defer d.lib.mu.Unlock()

	if d.lib.index != nil && time.Since(d.lib.builtAt) < libraryTTL {
		return d.lib.index
	}
	ix := d.buildLibraryIndex(ctx)
	d.lib.index, d.lib.builtAt = ix, time.Now()
	return ix
}

// libraryParents collects the DISTINCT parent folders across enabled sources.
// Sources commonly share them, and a shared parent must be listed once, not once
// per source (FR-007). A parent serving both movies and TV — an operator is free
// to point both at one folder — is folded into a single entry carrying both
// flags rather than appearing twice.
func libraryParents(providers []store.SourceProvider) []library.Parent {
	byPath := map[string]*library.Parent{}
	var order []string
	add := func(path string, movies bool) {
		path = strings.Trim(strings.TrimSpace(path), "/")
		if path == "" {
			return // a source with no parent configured contributes nothing
		}
		p, seen := byPath[path]
		if !seen {
			p = &library.Parent{Path: path}
			byPath[path] = p
			order = append(order, path)
		}
		if movies {
			p.Movies = true
		} else {
			p.TV = true
		}
	}
	for _, pr := range providers {
		if !pr.Enabled {
			continue
		}
		add(pr.MoviesParent, true)
		add(pr.TVParent, false)
	}
	out := make([]library.Parent, 0, len(order))
	for _, path := range order {
		out = append(out, *byPath[path])
	}
	return out
}

// buildLibraryIndex does the actual reading. Caller holds the cache lock.
func (d Deps) buildLibraryIndex(ctx context.Context) *library.Index {
	providers, err := d.Store.ListProviders()
	if err != nil {
		return library.Empty(time.Now())
	}
	parents := libraryParents(providers)
	if len(parents) == 0 {
		return library.Empty(time.Now())
	}

	names := make(map[string][]string, len(parents))
	err = d.NAS.Do(ctx, func(c syno.Client, sid string) error {
		for _, p := range parents {
			folders, e := c.ListFolder(ctx, sid, "/"+p.Path)
			if e != nil {
				// One unreadable parent must not blank out the others: a user may
				// have a working movies folder and a mistyped TV one. Skip it and
				// keep whatever else we can see.
				//
				// Nothing about the failure is logged — a DSM error can carry the
				// path, and folder names are NAS content (FR-026).
				continue
			}
			for _, f := range folders {
				names[p.Path] = append(names[p.Path], f.Name)
			}
		}
		return nil
	})
	if err != nil {
		// The NAS is unreachable or the session cannot be established at all.
		return library.Empty(time.Now())
	}
	return library.Build(parents, names, time.Now())
}

// mediaKind maps the catalog's own type strings onto the library's two parents.
// Series and anime both live under the TV parent, exactly as handleSourceSend
// already decides where to put them.
func mediaKind(catalogType string) library.MediaKind {
	switch catalogType {
	case "series", "anime":
		return library.MediaTV
	default:
		return library.MediaMovie
	}
}

// folderEvidence reports whether a title folder actually holds video.
//
// ok is false when the folder could not be read. That is deliberately distinct
// from "holds nothing": a failed read must never be reported as "you do not have
// this" (FR-009, FR-010c), so the caller shows no marker rather than a wrong one.
func (d Deps) folderEvidence(ctx context.Context, relPath string) (hasVideo, ok bool) {
	if d.lib == nil || d.NAS == nil || relPath == "" {
		return false, false
	}
	d.lib.mu.Lock()
	if rec, found := d.lib.evidence[relPath]; found && time.Since(rec.checkedAt) < libraryTTL {
		d.lib.mu.Unlock()
		return rec.hasVideo, true
	}
	d.lib.mu.Unlock()

	abs := "/" + relPath
	found := false
	err := d.NAS.Do(ctx, func(c syno.Client, sid string) error {
		files, e := c.ListFiles(ctx, sid, abs)
		if e != nil {
			return e
		}
		for _, name := range files {
			if library.IsVideo(name) {
				found = true
				return nil
			}
		}
		// Nothing at this level. A series keeps its episodes in season folders, so
		// one level down is still this title's content (FR-015).
		dirs, e := c.ListFolder(ctx, sid, abs)
		if e != nil {
			return e
		}
		for i, dir := range dirs {
			if i >= maxSeasonScan {
				break
			}
			sub, se := c.ListFiles(ctx, sid, abs+"/"+dir.Name)
			if se != nil {
				// One unreadable season must not condemn the whole title.
				continue
			}
			for _, name := range sub {
				if library.IsVideo(name) {
					found = true
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		// Nothing is logged: a DSM error can carry the path, and folder and file
		// names are NAS content (FR-026).
		return false, false
	}

	d.lib.mu.Lock()
	if d.lib.evidence == nil {
		d.lib.evidence = map[string]evidenceRec{}
	}
	d.lib.evidence[relPath] = evidenceRec{hasVideo: found, checkedAt: time.Now()}
	d.lib.mu.Unlock()
	return found, true
}

// ownershipOf resolves one catalog title against the library.
//
// Order matters. A title being written to is reported as downloading even though a
// partial video file is already on disk (FR-001b): "you have this" means skip it,
// "downloading" means wait, and only the second is true while bytes are arriving.
func (d Deps) ownershipOf(
	ctx context.Context, ix *library.Index, title string, kind library.MediaKind,
	activeDests map[string]bool,
) string {
	entry, matched := ix.Lookup(title, kind)
	if !matched {
		// No folder could be this title. Conclusive, and it cost no NAS read —
		// which is what keeps a page of unowned titles free (FR-010b).
		return source.OwnershipAbsent
	}
	if activeDests[entry.Path] {
		return source.OwnershipDownloading
	}
	hasVideo, ok := d.folderEvidence(ctx, entry.Path)
	if !ok {
		return source.OwnershipUnknown
	}
	if hasVideo {
		return source.OwnershipOwned
	}
	// The folder is there and holds no video: created ahead of a download, or left
	// behind holding only metadata. Not owned (FR-001a).
	return source.OwnershipAbsent
}

// activeDestinations is the watcher's view of what is being written to now.
func (d Deps) activeDestinations() map[string]bool {
	if d.ActiveDests == nil {
		return nil
	}
	return d.ActiveDests()
}

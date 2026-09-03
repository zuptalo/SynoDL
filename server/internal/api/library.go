package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/library"
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
}

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
	d.lib.index, d.lib.builtAt = nil, time.Time{}
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

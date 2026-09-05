package api

import (
	"context"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// libraryRetryTTL is how long a FAILED reading is held before trying again. Short
// on purpose: an empty snapshot from a failure looks exactly like "you own
// nothing", and holding that for the full TTL turns one blip into five minutes of
// missing markers for every user at once.
const libraryRetryTTL = 15 * time.Second

// libraryBuildTimeout bounds the detached build. Generous enough for a NAS listing
// a few large parents, short enough that a wedged NAS cannot pin the lock.
const libraryBuildTimeout = 20 * time.Second

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
	// builtOK is false when the last build could not reach the NAS, so the empty
	// snapshot it produced is retried in seconds rather than held for the TTL.
	builtOK bool

	// evidence is layer two: what a SPECIFIC title folder was found to contain.
	// Layer one (index) only says a folder with a matching name exists, which
	// 0.3.0 mistook for proof the content was there. Populated lazily and only for
	// folders backing a title actually being shown (FR-010b), so a page of titles
	// that match nothing costs no NAS read at all.
	evidence map[string]evidenceRec

	// invalidatedAt is when the reading was last declared out of date — a send, a
	// source's folders changing. A STORED reading older than this must not be
	// served as current, or keeping readings across restarts would quietly undo
	// every invalidation: the memory layer is dropped and the store answers with
	// exactly the reading that was just invalidated.
	//
	// It does not stop a stored reading being used as the NAS-is-down fallback.
	// Stale is a poor answer; "we cannot say" is a worse one.
	invalidatedAt time.Time

	// scanQueue is the folders asked for out of turn — a download landed there, or
	// one was just sent — so the background scan reads them at the front of its
	// next cycle instead of behind the rest of the library (spec 0011).
	scanQueue map[string]bool

	// titleNames remembers the catalog title behind a qualified title id, learned
	// when a search passed through this server.
	//
	// The title endpoint needs a title to match against the library, and the
	// drivers do not return one. The alternatives were worse: trusting a title
	// supplied by the client would let a user ask about anything, which is the
	// catalog-narrowing bypass FR-025c forbids; and parsing it out of each site's
	// page is driver work that would drift per provider. Learning it from the
	// user's OWN results means detail is only ever available for titles their
	// catalog actually returned.
	titleNames map[string]titleNameRec
}

// titleNameRec is one remembered catalog title.
type titleNameRec struct {
	title string
	kind  library.MediaKind
	at    time.Time
}

// maxRememberedTitles bounds the map. Discover pages are ~40 items, so this holds
// a long browsing session; past it the map is dropped wholesale rather than
// evicted one by one, because repopulating costs one search and the alternative
// is bookkeeping nobody will maintain.
const maxRememberedTitles = 2000

// evidenceRec is one folder's answer, with its own expiry so a folder checked
// long ago is re-read even while the name index is still warm.
type evidenceRec struct {
	hasVideo bool
	seasons  []seasonPresence
	// releases is which encodes were found, keyed by season — season 0 meaning the
	// title folder itself, so a movie is carried by the same field. It never
	// leaves the server: options are matched here and the client is told only
	// which option it already has (spec 1025, FR-006).
	releases map[int][]library.Release
	// folder is where this title actually lives on the NAS. Downloads are recorded
	// against a destination, so this is what matches them to a title — and unlike a
	// catalog id it is the same folder whichever source the user is browsing.
	folder string
	// keys is the identity of each file found, by season — the same shape as
	// releases, but the whole-file identity rather than tokens read out of it.
	// It is what a source that rewrites its release names leaves us to match on.
	keys      map[int][]string
	checkedAt time.Time
}

// seasonPresence is what a season folder actually holds.
//
// There is no Total and no Complete field, deliberately (FR-016a): the catalog's
// episode count cannot be relied on, and asserting completeness we cannot verify
// is the same over-claiming that FR-001a exists to prevent. VideoFiles > 0 with an
// empty Episodes is valid and means "present, numbering unreadable" (FR-016b).
type seasonPresence = source.SeasonPresence

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
	// Remembered titles are NOT dropped here: they record what the user saw, not
	// what the NAS holds, and a configuration change does not unsee them.
	d.lib.index, d.lib.builtAt, d.lib.evidence = nil, time.Time{}, nil
	d.lib.invalidatedAt = time.Now()
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

	ttl := libraryTTL
	if !d.lib.builtOK {
		// The last build failed. Holding an empty snapshot for the full TTL means
		// one blip suppresses every ownership marker, for every user, for five
		// minutes — so a failure is retried far sooner than a success is refreshed.
		ttl = libraryRetryTTL
	}
	if d.lib.index != nil && time.Since(d.lib.builtAt) < ttl {
		return d.lib.index
	}
	// Detached from the caller's context, with a bound of its own.
	//
	// This snapshot is instance-wide and shared. Building it on the REQUEST
	// context meant a single client hanging up mid-response cancelled the NAS
	// listing, which then cached as "the NAS holds nothing" and blanked ownership
	// for everyone until the TTL expired. A slow source makes clients hang up, so
	// the two failures compounded: the day one source went down, ownership
	// markers went with it.
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), libraryBuildTimeout)
	defer cancel()
	ix, ok := d.buildLibraryIndex(bctx)
	if !ok {
		// We could not ask the NAS. Falling back to an EMPTY reading is what used
		// to happen, and it is indistinguishable to a user from "you own nothing":
		// one blip blanked every ownership marker at once. The last good reading is
		// stale, but it is a far better answer than a wrong one (spec 0011 FR-004).
		if stored, found := d.storedLibraryIndex(); found {
			ix = stored
		}
	}
	d.lib.index, d.lib.builtAt, d.lib.builtOK = ix, time.Now(), ok
	return ix
}

// storedLibraryIndex rebuilds the index from the last reading written to the
// store. found is false when nothing has ever been stored, which is a real
// answer: the caller then behaves exactly as it did before this existed.
//
// The index is rebuilt from the reading's INPUTS rather than deserialised, so
// the matching rules stay in library.Build and a change to them needs no
// migration.
func (d Deps) storedLibraryIndex() (*library.Index, bool) {
	if d.Store == nil {
		return nil, false
	}
	f, err := d.Store.GetLibraryFolders()
	if err != nil || len(f.Parents) == 0 {
		return nil, false
	}
	parents := make([]library.Parent, 0, len(f.Parents))
	for _, p := range f.Parents {
		parents = append(parents, library.Parent{Path: p.Path, Movies: p.Movies, TV: p.TV})
	}
	return library.Build(parents, f.Names, f.ScannedAt), true
}

// persistLibraryFolders writes a successful reading through to the store, so the
// next start-up and the next blip have something to fall back on.
//
// Wholesale, and only on success: a partial or failed reading would delete rows
// for parents it simply could not see, which is the opposite of the point.
// A failure to write is ignored — the in-memory reading is still good, and
// nothing about the folders may be logged (FR-012).
func (d Deps) persistLibraryFolders(parents []library.Parent, names map[string][]string, at time.Time) {
	if d.Store == nil {
		return
	}
	sp := make([]store.LibraryParent, 0, len(parents))
	for _, p := range parents {
		sp = append(sp, store.LibraryParent{Path: p.Path, Movies: p.Movies, TV: p.TV})
	}
	_ = d.Store.SaveLibraryFolders(store.LibraryFolders{Parents: sp, Names: names, ScannedAt: at})
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
// buildLibraryIndex reads the configured parents from the NAS.
//
// ok distinguishes "the NAS says there is nothing here" from "we could not ask".
// Both yield an empty index and neither is shown to the user, but only the second
// is worth retrying promptly.
func (d Deps) buildLibraryIndex(ctx context.Context) (*library.Index, bool) {
	providers, err := d.Store.ListProviders()
	if err != nil {
		return library.Empty(time.Now()), false
	}
	parents := libraryParents(providers)
	if len(parents) == 0 {
		// Nothing configured is a real answer, and it must reach the store too:
		// a stored reading that outlived the source it describes would keep
		// answering for folders the operator has disconnected (FR-009).
		//
		// Only when there IS something to clear, though. An instance with no
		// sources — every one before the operator adds the first — would otherwise
		// run a write transaction on every build, for nothing.
		if stored, err := d.Store.GetLibraryFolders(); err == nil && len(stored.Parents) > 0 {
			d.persistLibraryFolders(nil, nil, time.Now())
		}
		return library.Empty(time.Now()), true
	}

	names := make(map[string][]string, len(parents))
	// listed counts parents we actually got an answer for — including an answer of
	// "this folder is empty", which is a real reading. It is NOT the same as
	// len(names): a configured parent that genuinely holds nothing contributes no
	// names but is still a successful read.
	listed := 0
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
			listed++
			for _, f := range folders {
				names[p.Path] = append(names[p.Path], f.Name)
			}
		}
		return nil
	})
	if err != nil {
		// The NAS is unreachable or the session cannot be established at all.
		return library.Empty(time.Now()), false
	}
	if listed == 0 {
		// Every configured parent failed to list. The session was established, so
		// nothing above reported an error — but this is emphatically not a reading
		// of an empty NAS, and treating it as one had two costs: it was held for
		// the full success TTL rather than retried in seconds, and now that a good
		// reading is kept, it would overwrite that too.
		return library.Empty(time.Now()), false
	}
	at := time.Now()
	// Kept, so a restart and a blip both have a last good reading to fall back on
	// (spec 0011 FR-001/FR-003/FR-004).
	d.persistLibraryFolders(parents, names, at)
	return library.Build(parents, names, at), true
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
// folderEvidence reports what a title folder actually holds: whether there is any
// video, and for a series which seasons and episodes are present.
//
// ok is false when the folder could not be read. That is deliberately distinct
// from "holds nothing": a failed read must never be reported as "you do not have
// this" (FR-009, FR-010c), so the caller shows no marker rather than a wrong one.
func (d Deps) folderEvidence(ctx context.Context, relPath string) (evidenceRec, bool) {
	if d.lib == nil || d.NAS == nil || relPath == "" {
		return evidenceRec{}, false
	}
	d.lib.mu.Lock()
	if rec, found := d.lib.evidence[relPath]; found && time.Since(rec.checkedAt) < libraryTTL {
		d.lib.mu.Unlock()
		return rec, true
	}
	d.lib.mu.Unlock()

	// Nothing in memory. Before paying for a NAS read, see whether a previous run
	// already answered for this folder (spec 0011 FR-003). A fresh-enough stored
	// reading is returned as-is, so the first person to open a title after a
	// deploy waits on nothing.
	if rec, found := d.storedFolderEvidence(relPath); found &&
		time.Since(rec.checkedAt) < libraryTTL && d.storedReadingIsCurrent(rec) {
		d.rememberEvidence(relPath, rec)
		return rec, true
	}

	abs := "/" + relPath
	rec := evidenceRec{}
	// Episodes are collected per season as they are found, so a season stored
	// flat and one stored in its own folder converge on the same shape.
	bySeason := map[int]map[int]bool{}
	counts := map[int]int{}

	rec.releases = map[int][]library.Release{}
	rec.keys = map[int][]string{}

	note := func(season, episode int, ok bool) {
		if !ok {
			return
		}
		if bySeason[season] == nil {
			bySeason[season] = map[int]bool{}
		}
		bySeason[season][episode] = true
	}

	// noteRelease remembers WHICH encode a file is, from the name already in hand.
	// Nothing here costs an extra NAS call, and the name itself is dropped — only
	// the two tokens that identify the release are kept.
	noteRelease := func(season int, name string) {
		rel, ok := library.ReleaseOf(name)
		// The whole-file identity is recorded whether or not the tokens could be
		// read: a source that renames what it serves leaves nothing else to go on.
		if rel.Key != "" {
			if !slices.Contains(rec.keys[season], rel.Key) {
				rec.keys[season] = append(rec.keys[season], rel.Key)
			}
		}
		if !ok {
			return
		}
		for _, have := range rec.releases[season] {
			if have == rel {
				return // one entry per distinct release, not one per episode
			}
		}
		rec.releases[season] = append(rec.releases[season], rel)
	}

	err := d.NAS.Do(ctx, func(c syno.Client, sid string) error {
		files, e := c.ListFiles(ctx, sid, abs)
		if e != nil {
			return e
		}
		// Episodes sitting directly in the title folder (FR-015, flat layout).
		for _, name := range files {
			if !library.IsVideo(name) {
				continue
			}
			rec.hasVideo = true
			if season, episode, ok := library.EpisodeOf(name); ok {
				counts[season]++
				note(season, episode, true)
				noteRelease(season, name)
				continue
			}
			// A movie sits in the title folder with no season at all; season 0 is
			// where its releases live.
			noteRelease(0, name)
		}

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
			// The season number comes from the FILES where they say, and from the
			// folder name only as a fallback — the files are what actually landed.
			folderSeason, folderOK := library.SeasonOfFolder(dir.Name)
			for _, name := range sub {
				if !library.IsVideo(name) {
					continue
				}
				rec.hasVideo = true
				season, episode, ok := library.EpisodeOf(name)
				if !ok && folderOK {
					season, ok = folderSeason, false
					counts[season]++
					continue
				}
				if !ok {
					continue
				}
				counts[season]++
				note(season, episode, true)
				noteRelease(season, name)
			}
			// A season folder holding video whose names say nothing is still
			// present (FR-016b) — record it so it is not silently dropped.
			if folderOK && counts[folderSeason] == 0 {
				for _, name := range sub {
					if library.IsVideo(name) {
						counts[folderSeason]++
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		// Nothing is logged: a DSM error can carry the path, and folder and file
		// names are NAS content (FR-026).
		//
		// A stored reading is a far better answer than "we cannot say", which is
		// what blanks a title's season detail the moment the NAS hiccups. Age is
		// bounded so a folder nobody has looked at in a week is not asserted on
		// the strength of a week-old reading (FR-004).
		if rec, found := d.storedFolderEvidence(relPath); found &&
			time.Since(rec.checkedAt) < storedEvidenceMaxAge {
			d.rememberEvidence(relPath, rec)
			return rec, true
		}
		return evidenceRec{}, false
	}

	for season, n := range counts {
		eps := make([]int, 0, len(bySeason[season]))
		for e := range bySeason[season] {
			eps = append(eps, e)
		}
		sort.Ints(eps)
		rec.seasons = append(rec.seasons, seasonPresence{Season: season, Episodes: eps, VideoFiles: n})
	}
	sort.Slice(rec.seasons, func(i, j int) bool { return rec.seasons[i].Season < rec.seasons[j].Season })
	rec.checkedAt = time.Now()

	d.rememberEvidence(relPath, rec)
	d.persistFolderEvidence(relPath, rec)
	return rec, true
}

// storedReadingIsCurrent reports whether a stored reading predates the last
// invalidation. One that does describes the NAS as it was BEFORE whatever made
// us drop the reading — most often a download this user just sent, which is
// exactly the case they are watching.
func (d Deps) storedReadingIsCurrent(rec evidenceRec) bool {
	d.lib.mu.Lock()
	defer d.lib.mu.Unlock()
	return rec.checkedAt.After(d.lib.invalidatedAt)
}

// rememberEvidence puts one folder's reading in the in-memory layer.
func (d Deps) rememberEvidence(relPath string, rec evidenceRec) {
	d.lib.mu.Lock()
	if d.lib.evidence == nil {
		d.lib.evidence = map[string]evidenceRec{}
	}
	d.lib.evidence[relPath] = rec
	d.lib.mu.Unlock()
}

// storedEvidenceMaxAge bounds how old a STORED folder reading may be and still
// be offered as a fallback when the NAS cannot be reached. A week is long enough
// to cover any outage worth surviving, and short enough that a folder nobody has
// looked at since is re-read before it is trusted.
const storedEvidenceMaxAge = 7 * 24 * time.Hour

// storedFolderEvidence reads one folder's last recorded answer back out of the
// store. found is false when there is none, which is a real answer: the caller
// then reads the NAS exactly as it did before any of this existed.
func (d Deps) storedFolderEvidence(relPath string) (evidenceRec, bool) {
	if d.Store == nil || relPath == "" {
		return evidenceRec{}, false
	}
	e, found, err := d.Store.GetLibraryEvidence(relPath)
	if err != nil || !found {
		return evidenceRec{}, false
	}
	rec := evidenceRec{
		hasVideo:  e.HasVideo,
		folder:    relPath,
		checkedAt: e.CheckedAt,
		releases:  map[int][]library.Release{},
		keys:      map[int][]string{},
	}
	for _, s := range e.Seasons {
		rec.seasons = append(rec.seasons, source.SeasonPresence{
			Season: s.Season, Episodes: s.Episodes, VideoFiles: s.VideoFiles,
		})
	}
	for season, rels := range e.Releases {
		for _, r := range rels {
			rec.releases[season] = append(rec.releases[season],
				library.Release{Resolution: r.Resolution, Group: r.Group, Key: r.Key})
		}
	}
	for season, ks := range e.FileKeys {
		rec.keys[season] = append(rec.keys[season], ks...)
	}
	return rec, true
}

// persistFolderEvidence keeps a successful reading, so the next start-up and the
// next blip have it. A write failure is ignored: the in-memory reading is still
// good, and nothing about the folder may be logged (FR-012).
func (d Deps) persistFolderEvidence(relPath string, rec evidenceRec) {
	if d.Store == nil || relPath == "" {
		return
	}
	e := store.LibraryEvidence{
		Path:      relPath,
		HasVideo:  rec.hasVideo,
		Releases:  map[int][]store.ReleaseToken{},
		FileKeys:  map[int][]string{},
		CheckedAt: rec.checkedAt,
	}
	for _, s := range rec.seasons {
		e.Seasons = append(e.Seasons, store.SeasonPresence{
			Season: s.Season, Episodes: s.Episodes, VideoFiles: s.VideoFiles,
		})
	}
	for season, rels := range rec.releases {
		for _, r := range rels {
			e.Releases[season] = append(e.Releases[season],
				store.ReleaseToken{Resolution: r.Resolution, Group: r.Group, Key: r.Key})
		}
	}
	for season, ks := range rec.keys {
		e.FileKeys[season] = append(e.FileKeys[season], ks...)
	}
	_ = d.Store.SaveLibraryEvidence(e)
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
	rec, ok := d.folderEvidence(ctx, entry.Path)
	if !ok {
		return source.OwnershipUnknown
	}
	if rec.hasVideo {
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

// titleDetail answers "what of this title is already here" for one catalog title.
//
// Returns the ownership state and, for a series, the seasons actually present.
// A movie gets no season breakdown (FR-018), and a folder that cannot be read
// yields unknown with no seasons so the download options stay fully usable
// (FR-017).
func (d Deps) titleDetail(
	ctx context.Context, title string, kind library.MediaKind, activeDests map[string]bool,
) (string, []source.SeasonPresence, evidenceRec) {
	ix := d.libraryIndex(ctx)
	if ix.IsEmpty() {
		return source.OwnershipUnknown, nil, evidenceRec{}
	}
	entry, matched := ix.Lookup(title, kind)
	if !matched {
		return source.OwnershipAbsent, nil, evidenceRec{}
	}
	if activeDests[entry.Path] {
		// Still arriving. Season detail from a half-written folder would be read
		// as what the user HAS, so it is withheld rather than shown mid-flight —
		// and so are the releases, for the same reason.
		return source.OwnershipDownloading, nil, evidenceRec{}
	}
	rec, ok := d.folderEvidence(ctx, entry.Path)
	if !ok {
		return source.OwnershipUnknown, nil, evidenceRec{}
	}
	if !rec.hasVideo {
		return source.OwnershipAbsent, nil, evidenceRec{}
	}
	rec.folder = entry.Path
	if kind == library.MediaMovie {
		return source.OwnershipOwned, nil, rec
	}
	return source.OwnershipOwned, rec.seasons, rec
}

// markOwnedOptions flags the options whose release is the one already on the NAS.
//
// Both halves must agree — the resolution AND the group that produced the encode
// — because a season's options routinely share a resolution and differ only by
// encoder. Marking on a resolution match was the bug this replaces: it stamped
// every option for a season the user had, which is wrong for all but one of them.
//
// An option nothing matches is simply left unmarked. That is not a claim the
// user lacks it: the season's own presence is reported separately and is
// untouched by whether any release could be identified (FR-004).
func markOwnedOptions(qualities []source.QualityOption, ev evidenceRec, sent []store.SourceDownload) []source.QualityOption {
	// What WE sent is the most reliable answer there is, and often the only one.
	// A library renamed for a media server keeps no release information in its
	// file names — no resolution, no group — so reading them back can identify
	// nothing. Our own record survives any renaming (spec 0010).
	for i := range qualities {
		if versionWasSent(qualities[i], sent) {
			qualities[i].Owned = true
		}
	}
	if len(ev.releases) == 0 && len(ev.keys) == 0 {
		return qualities
	}
	for i := range qualities {
		if qualities[i].Owned {
			continue // already answered, and answered better
		}
		q := qualities[i]
		season := seasonNumOf(q.Season)

		// Where the option knows the file it produces, that decides it — and a
		// mismatch is final. Falling through to tokens here would undo the whole
		// point: on a source that rewrites its release names EVERY token
		// comparison succeeds, so the fallback would mark every option again
		// (FR-004).
		if key := library.ReleaseKey(q.ReleaseName); key != "" {
			qualities[i].Owned = slices.Contains(ev.keys[season], key)
			continue
		}

		if q.Encoder == "" || q.Resolution == "" {
			continue // an option that does not say what it is cannot be identified
		}
		for _, rel := range ev.releases[season] {
			if rel.Matches(q.Resolution, q.Encoder) {
				qualities[i].Owned = true
				break
			}
		}
	}
	return qualities
}

// versionWasSent reports whether this exact option is one we sent to this title.
//
// Matched on the option's identity where the source gives a stable one, and
// otherwise on how it describes itself. Both sides come from the same source, so
// a description that matched when it was sent matches when it is read back.
func versionWasSent(q source.QualityOption, sent []store.SourceDownload) bool {
	season := seasonNumOf(q.Season)
	for _, d := range sent {
		// A series records one row per season folder, so the season has to agree;
		// a movie records the title folder and both sides are season 0.
		if seasonOfDestination(d.Destination) != season {
			continue
		}
		// A record written by a send carries the option's own wording, so it is
		// compared whole. Ids are positional on at least one source, so a listing
		// that reorders makes an id point at a different option entirely —
		// matching on it would mark the wrong version with full confidence, which
		// is worse than marking nothing.
		if d.QualityLabel != "" {
			if d.QualityLabel == q.Label &&
				d.QualityResolution == q.Resolution &&
				d.QualityEncoder == q.Encoder {
				return true
			}
			continue // described, and this is not it
		}
		// A record RECOVERED from the name we downloaded carries only the two
		// tokens, and its encoder is already folded for comparison.
		if d.QualityResolution != "" && d.QualityEncoder != "" {
			rel := library.Release{Resolution: d.QualityResolution, Group: d.QualityEncoder}
			if rel.Matches(q.Resolution, q.Encoder) {
				return true
			}
			continue
		}
		// Only a record with nothing but an id falls back to it.
		if d.QualityID != "" && d.QualityID == q.ID {
			return true
		}
	}
	return false
}

// seasonOfDestination reads the season out of a recorded destination folder.
// A movie has none, which is season 0 — the same number an option with no season
// carries, so the two line up without a special case.
func seasonOfDestination(dest string) int {
	dest = strings.Trim(strings.TrimSpace(dest), "/")
	if dest == "" {
		return 0
	}
	parts := strings.Split(dest, "/")
	if n, ok := library.SeasonOfFolder(parts[len(parts)-1]); ok {
		return n
	}
	return 0
}

// reSeasonNum pulls the season number out of a source's own season label, which
// may be in any language ("Season 2", "فصل 2") but writes the number in western
// digits. Mirrors seasonNum() in the client's quality-sort.ts.
var reSeasonNum = regexp.MustCompile(`(\d+)`)

// seasonNumOf is 0 for an option with no season — a movie, whose releases are
// recorded under season 0 too.
func seasonNumOf(label string) int {
	m := reSeasonNum.FindStringSubmatch(label)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// rememberTitle records the catalog title behind a qualified id, so the title
// endpoint can match it against the library later.
func (d Deps) rememberTitle(id, title string, kind library.MediaKind) {
	if d.lib == nil || id == "" || strings.TrimSpace(title) == "" {
		return
	}
	d.lib.mu.Lock()
	defer d.lib.mu.Unlock()
	if d.lib.titleNames == nil || len(d.lib.titleNames) > maxRememberedTitles {
		d.lib.titleNames = map[string]titleNameRec{}
	}
	d.lib.titleNames[id] = titleNameRec{title: title, kind: kind, at: time.Now()}
}

// rememberedTitle returns what was learned about a qualified id, if anything.
func (d Deps) rememberedTitle(id string) (titleNameRec, bool) {
	if d.lib == nil {
		return titleNameRec{}, false
	}
	d.lib.mu.Lock()
	defer d.lib.mu.Unlock()
	rec, ok := d.lib.titleNames[id]
	return rec, ok
}

package api

import (
	"context"
	"errors"
	"path/filepath"
	"synodl/server/internal/source"
	"testing"
	"time"

	"synodl/server/internal/config"
	"synodl/server/internal/library"
	"synodl/server/internal/nas"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// libDeps builds stateful Deps whose NAS calls all land on one fake client, so
// the snapshot logic can be exercised with no mock DSM and no HTTP.
func libDeps(t *testing.T, fake *fakeSyno) (Deps, *store.Store) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// A stored NAS connection so the manager can "log in" against the fake.
	if err := st.SaveOperatorConfig(store.OperatorConfig{
		NASAddress: "nas.local", NASPort: 5001, NASTLSVerify: true,
		NASAccount: "admin", NASPassword: "pw",
	}); err != nil {
		t.Fatalf("SaveOperatorConfig: %v", err)
	}
	factory := func(base string, insecure bool) syno.Client { return fake }
	return Deps{
		Cfg:      config.Config{MaxTorrentMB: 16, LoginPerMinute: 1000},
		Stateful: true,
		Store:    st,
		NAS:      nas.New(st, factory),
		lib:      &libraryCache{},
	}, st
}

func addSource(t *testing.T, st *store.Store, name, movies, tv string) {
	t.Helper()
	if _, err := st.CreateProvider(store.SourceProvider{
		Kind: "faketest", DisplayName: name, Enabled: true,
		MoviesParent: movies, TVParent: tv, State: store.SourceActive,
	}, time.Now().Unix()); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
}

// FR-007: sources commonly share parents, and listing the same folder once per
// source would multiply NAS calls for no gain.
func TestLibrarySnapshotListsEachDistinctParentOnce(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie":   {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
			"/tv-show": {{Name: "Friends", Path: "/tv-show/Friends"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	addSource(t, st, "Beta", "movie", "tv-show") // same parents

	ix := d.libraryIndex(context.Background())
	if ix.IsEmpty() {
		t.Fatal("snapshot is empty")
	}
	if n := fake.folderListCalls; n != 2 {
		t.Errorf("listed %d folders, want 2 (one per DISTINCT parent, not per source)", n)
	}
	if _, ok := ix.Lookup("Dune 2021", library.MediaMovie); !ok {
		t.Error("movie not found in the snapshot")
	}
	if _, ok := ix.Lookup("Friends", library.MediaTV); !ok {
		t.Error("series not found in the snapshot")
	}
}

// FR-009: a NAS failure must degrade to "nothing is present", never to an error.
func TestLibrarySnapshotDegradesToEmptyOnNASError(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid", err: errors.New("nas unreachable")}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	ix := d.libraryIndex(context.Background())
	if !ix.IsEmpty() {
		t.Fatal("a failed read must produce an empty snapshot")
	}
	if _, ok := ix.Lookup("Dune 2021", library.MediaMovie); ok {
		t.Error("empty snapshot matched a title")
	}
}

func TestLibrarySnapshotIsEmptyWithNoSourceConfigured(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, _ := libDeps(t, fake)

	if ix := d.libraryIndex(context.Background()); !ix.IsEmpty() {
		t.Error("no configured source should yield an empty snapshot")
	}
	if fake.folderListCalls != 0 {
		t.Error("no source configured, so the NAS should not be touched at all")
	}
}

// FR-010: a reading younger than the TTL is reused, so browsing adds no NAS
// round-trip; an older one is rebuilt.
func TestLibrarySnapshotReusesWithinTTLAndRebuildsAfter(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021"}}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	d.libraryIndex(context.Background())
	first := fake.folderListCalls
	d.libraryIndex(context.Background())
	if fake.folderListCalls != first {
		t.Errorf("a warm snapshot re-read the NAS: %d then %d calls", first, fake.folderListCalls)
	}

	// Age the snapshot past the TTL.
	d.lib.mu.Lock()
	d.lib.builtAt = time.Now().Add(-libraryTTL - time.Second)
	d.lib.mu.Unlock()

	d.libraryIndex(context.Background())
	if fake.folderListCalls <= first {
		t.Error("an expired snapshot was not rebuilt")
	}
}

// FR-008: a title the user just sent must be marked at once, not after the TTL.
// FR-008a: a configuration change must not leave a snapshot answering for
// folders that are no longer the configured ones.
func TestLibraryInvalidateForcesARebuild(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021"}}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	d.libraryIndex(context.Background())
	before := fake.folderListCalls

	d.invalidateLibrary()

	// A title that was not there when the first snapshot was taken.
	fake.subfolders["/movie"] = append(fake.subfolders["/movie"], syno.Folder{Name: "Arrival 2016"})
	ix := d.libraryIndex(context.Background())
	if fake.folderListCalls <= before {
		t.Fatal("invalidation did not force a rebuild")
	}
	if _, ok := ix.Lookup("Arrival 2016", library.MediaMovie); !ok {
		t.Error("the rebuilt snapshot does not see the newly added title")
	}
}

// A disabled source's parents are not part of the configured set.
func TestLibrarySnapshotIgnoresDisabledSources(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, st := libDeps(t, fake)
	if _, err := st.CreateProvider(store.SourceProvider{
		Kind: "faketest", DisplayName: "Off", Enabled: false,
		MoviesParent: "movie", TVParent: "tv-show", State: store.SourceActive,
	}, time.Now().Unix()); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if ix := d.libraryIndex(context.Background()); !ix.IsEmpty() {
		t.Error("a disabled source should contribute no parents")
	}
	if fake.folderListCalls != 0 {
		t.Error("a disabled source should cause no NAS listing")
	}
}

// A source with no parents configured contributes nothing and must not cause a
// listing of the share root.
func TestLibrarySnapshotSkipsUnsetParents(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "", "")

	if ix := d.libraryIndex(context.Background()); !ix.IsEmpty() {
		t.Error("unset parents should contribute nothing")
	}
	if fake.folderListCalls != 0 {
		t.Errorf("unset parents caused %d NAS listings", fake.folderListCalls)
	}
}

// The defect this whole amendment exists to fix: a folder is not evidence.
//
// The operator's NAS had "Attack on Titan (2013)/Season 00" holding nothing but
// season.nfo, and 0.3.0 reported the title as owned. Artwork, subtitles and .nfo
// legitimately sit beside content without being it (FR-001a).
func TestOwnershipRequiresAVideoFileNotJustAFolder(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {
				{Name: "Dune 2021", Path: "/movie/Dune 2021"},
				{Name: "Arrival 2016", Path: "/movie/Arrival 2016"},
				{Name: "Empty 2020", Path: "/movie/Empty 2020"},
			},
		},
		files: map[string][]string{
			"/movie/Dune 2021":    {"poster.jpg", "Dune.2021.mkv"},
			"/movie/Arrival 2016": {"poster.jpg", "Arrival.srt", "movie.nfo"},
			"/movie/Empty 2020":   {},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	ix := d.libraryIndex(context.Background())

	for _, c := range []struct{ title, want string }{
		{"Dune 2021", source.OwnershipOwned},
		{"Arrival 2016", source.OwnershipAbsent}, // sidecars only
		{"Empty 2020", source.OwnershipAbsent},   // folder made ahead of a download
		{"Nothing Here 1999", source.OwnershipAbsent},
	} {
		got := d.ownershipOf(context.Background(), ix, c.title, library.MediaMovie, nil)
		if got != c.want {
			t.Errorf("ownershipOf(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

// A series keeps its episodes one level down, and that is still this title's
// content (FR-015).
func TestOwnershipFindsVideoInSeasonSubfolders(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":              {{Name: "Friends 1994", Path: "/tv-show/Friends 1994"}},
			"/tv-show/Friends 1994": {{Name: "Season 01", Path: "/tv-show/Friends 1994/Season 01"}},
		},
		files: map[string][]string{
			"/tv-show/Friends 1994":           {"poster.jpg"},
			"/tv-show/Friends 1994/Season 01": {"Friends.S01E01.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	ix := d.libraryIndex(context.Background())
	if got := d.ownershipOf(context.Background(), ix, "Friends 1994", library.MediaTV, nil); got != source.OwnershipOwned {
		t.Errorf("ownershipOf(Friends) = %q, want owned — the episodes are in Season 01", got)
	}
}

// FR-010b: the cost model. Most titles on a page match no folder, and those must
// cost nothing — this is the difference between the lazy design and the eager scan
// the plan rejected, and nothing in the UI would reveal a regression in it.
func TestTitlesThatMatchNoFolderCostNoNASRead(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files:      map[string][]string{"/movie/Dune 2021": {"Dune.2021.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	ix := d.libraryIndex(context.Background())

	before := fake.fileListCalls
	for _, title := range []string{"Nope 1999", "Also Nope 2001", "Still Nothing 2015"} {
		d.ownershipOf(context.Background(), ix, title, library.MediaMovie, nil)
	}
	if fake.fileListCalls != before {
		t.Errorf("%d folder listings for titles that match nothing, want 0", fake.fileListCalls-before)
	}

	// And a match is verified once, then answered from the cache.
	d.ownershipOf(context.Background(), ix, "Dune 2021", library.MediaMovie, nil)
	afterFirst := fake.fileListCalls
	d.ownershipOf(context.Background(), ix, "Dune 2021", library.MediaMovie, nil)
	if fake.fileListCalls != afterFirst {
		t.Error("a second lookup of the same title re-read the NAS instead of using the cache")
	}
}

// FR-009/FR-010c: a failed read is NOT "you do not have this". Reporting absent on
// an error would tell a user to download something they already own.
func TestUnreadableFolderIsUnknownNotAbsent(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	ix := d.libraryIndex(context.Background())

	// Fail only the per-folder read, after the index is already built.
	fake.err = errors.New("nas unreachable")
	if got := d.ownershipOf(context.Background(), ix, "Dune 2021", library.MediaMovie, nil); got != source.OwnershipUnknown {
		t.Errorf("ownershipOf with an unreadable folder = %q, want unknown", got)
	}
}

// FR-001b: Download Station writes the video into the destination as it goes, so a
// part-fetched title HAS a video file and would otherwise read as owned. The advice
// differs — skip it, versus wait for it.
func TestDownloadingOutranksOwned(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files:      map[string][]string{"/movie/Dune 2021": {"Dune.2021.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	ix := d.libraryIndex(context.Background())

	active := map[string]bool{"movie/Dune 2021": true}
	if got := d.ownershipOf(context.Background(), ix, "Dune 2021", library.MediaMovie, active); got != source.OwnershipDownloading {
		t.Errorf("ownershipOf while downloading = %q, want downloading", got)
	}
	// With nothing in flight the same title is simply owned.
	if got := d.ownershipOf(context.Background(), ix, "Dune 2021", library.MediaMovie, nil); got != source.OwnershipOwned {
		t.Errorf("ownershipOf with no active task = %q, want owned", got)
	}
}

// US2: which seasons are here, and which episodes each holds. The gap is the
// point — a user with episodes 1-6 and 9 should see that, not "season present".
func TestSeasonAndEpisodePresence(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":              {{Name: "Friends 1994", Path: "/tv-show/Friends 1994"}},
			"/tv-show/Friends 1994": {{Name: "Season 01"}, {Name: "Season 02"}, {Name: "Specials"}},
		},
		files: map[string][]string{
			"/tv-show/Friends 1994":           {"poster.jpg"},
			"/tv-show/Friends 1994/Season 01": {"F.S01E01.mkv", "F.S01E02.mkv", "F.S01E06.mkv", "sub.srt"},
			"/tv-show/Friends 1994/Season 02": {"F.S02E01.mkv"},
			"/tv-show/Friends 1994/Specials":  {"season.nfo"}, // no video: not present
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	own, seasons, _ := d.titleDetail(context.Background(), "Friends 1994", library.MediaTV, nil)
	if own != source.OwnershipOwned {
		t.Fatalf("ownership = %q, want owned", own)
	}
	got := map[int][]int{}
	for _, s := range seasons {
		got[s.Season] = s.Episodes
	}
	if len(got[1]) != 3 || got[1][0] != 1 || got[1][2] != 6 {
		t.Errorf("season 1 episodes = %v, want [1 2 6] — the gap at 3-5 is the whole point", got[1])
	}
	if len(got[2]) != 1 || got[2][0] != 1 {
		t.Errorf("season 2 episodes = %v, want [1]", got[2])
	}
	// A season folder holding only metadata is not present, exactly as a title
	// folder holding only metadata is not owned (FR-001a).
	if _, listed := got[0]; listed {
		t.Error("Specials holds no video and must not be listed as present")
	}
}

// FR-016b: a season whose file names say nothing is still present. Dropping it
// would report "you do not have this" for content that is right there.
func TestSeasonWithUnreadableNumberingIsStillPresent(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":           {{Name: "Show 2020", Path: "/tv-show/Show 2020"}},
			"/tv-show/Show 2020": {{Name: "Season 03"}},
		},
		files: map[string][]string{
			"/tv-show/Show 2020":           {},
			"/tv-show/Show 2020/Season 03": {"episode-one.mkv", "episode-two.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	own, seasons, _ := d.titleDetail(context.Background(), "Show 2020", library.MediaTV, nil)
	if own != source.OwnershipOwned {
		t.Fatalf("ownership = %q, want owned", own)
	}
	if len(seasons) != 1 || seasons[0].Season != 3 {
		t.Fatalf("seasons = %+v, want season 3 present", seasons)
	}
	if len(seasons[0].Episodes) != 0 {
		t.Errorf("episodes = %v, want none — the names say nothing", seasons[0].Episodes)
	}
	if seasons[0].VideoFiles != 2 {
		t.Errorf("videoFiles = %d, want 2", seasons[0].VideoFiles)
	}
}

// FR-018: a movie is present without a season breakdown.
func TestMovieHasNoSeasonBreakdown(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files:      map[string][]string{"/movie/Dune 2021": {"Dune.2021.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	own, seasons, _ := d.titleDetail(context.Background(), "Dune 2021", library.MediaMovie, nil)
	if own != source.OwnershipOwned || len(seasons) != 0 {
		t.Errorf("movie = (%q, %+v), want owned with no seasons", own, seasons)
	}
}

// Spec 1025 US1: a season on disk used to stamp EVERY option for that season as
// already downloaded — the 1080p one, the x265 one and the other encoder's alike.
// Only the release actually on disk may be marked.
func TestOnlyTheReleaseOnDiskIsMarked(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":           {{Name: "Show 2020", Path: "/tv-show/Show 2020"}},
			"/tv-show/Show 2020": {{Name: "Season 01"}},
		},
		files: map[string][]string{
			"/tv-show/Show 2020/Season 01": {
				"Show.S01E01.1080p.BluRay.x265-Silence.mkv",
				"Show.S01E02.1080p.BluRay.x265-Silence.mkv",
			},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	_, _, ev := d.titleDetail(context.Background(), "Show 2020", library.MediaTV, nil)
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Season: "Season 1", Resolution: "1080p", Encoder: "Silence"},
		{ID: "b", Season: "Season 1", Resolution: "1080p", Encoder: "TENEIGHTY"},
		{ID: "c", Season: "Season 1", Resolution: "720p", Encoder: "Silence"},
		{ID: "d", Season: "Season 2", Resolution: "1080p", Encoder: "Silence"},
	}, ev, nil)

	owned := map[string]bool{}
	for _, q := range got {
		owned[q.ID] = q.Owned
	}
	if !owned["a"] {
		t.Error("the release actually on disk must be marked")
	}
	if owned["b"] {
		t.Error("same resolution, different encoder — a resolution match alone must never mark (FR-002)")
	}
	if owned["c"] {
		t.Error("same encoder, different resolution — must not mark")
	}
	if owned["d"] {
		t.Error("a season that is not on disk must not be marked")
	}
}

// FR-003/FR-004: files that do not name a release identify nothing — but the
// season is still present, and saying otherwise would be the opposite mistake.
func TestUnidentifiableFilesMarkNothingButStayPresent(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":           {{Name: "Show 2020", Path: "/tv-show/Show 2020"}},
			"/tv-show/Show 2020": {{Name: "Season 01"}},
		},
		files: map[string][]string{
			// Real episodes, but nothing about how they were encoded.
			"/tv-show/Show 2020/Season 01": {"Show S01E01.mkv", "Show S01E02.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	own, seasons, ev := d.titleDetail(context.Background(), "Show 2020", library.MediaTV, nil)
	if own != source.OwnershipOwned {
		t.Fatalf("ownership = %q, want owned — an unidentifiable release is still a release", own)
	}
	if len(seasons) != 1 || len(seasons[0].Episodes) != 2 {
		t.Fatalf("season presence must be unaffected: %+v", seasons)
	}
	for _, q := range markOwnedOptions([]source.QualityOption{
		{ID: "a", Season: "Season 1", Resolution: "1080p", Encoder: "Silence"},
	}, ev, nil) {
		if q.Owned {
			t.Error("nothing may be marked when the files identify no release")
		}
	}
}

// A movie has no seasons; its releases live under season 0 and match options
// that carry no season at all.
func TestMovieReleaseIsMatched(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files: map[string][]string{
			"/movie/Dune 2021": {"Dune.2021.2160p.UHD.BluRay-FraMeSToR.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	_, _, ev := d.titleDetail(context.Background(), "Dune 2021", library.MediaMovie, nil)
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Resolution: "2160p", Encoder: "FraMeSToR"},
		{ID: "b", Resolution: "1080p", Encoder: "FraMeSToR"},
	}, ev, nil)
	if !got[0].Owned {
		t.Error("the movie release on disk must be marked")
	}
	if got[1].Owned {
		t.Error("a different resolution must not be marked")
	}
}

// A title still arriving reports as downloading and marks nothing: a half-written
// folder must not be read as what the user has.
func TestDownloadingTitleMarksNoRelease(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files:      map[string][]string{"/movie/Dune 2021": {"Dune.2021.2160p.UHD.BluRay-FraMeSToR.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	own, _, ev := d.titleDetail(context.Background(), "Dune 2021", library.MediaMovie,
		map[string]bool{"movie/Dune 2021": true})
	if own != source.OwnershipDownloading {
		t.Fatalf("ownership = %q, want downloading", own)
	}
	if len(ev.releases) != 0 || len(ev.keys) != 0 {
		t.Fatalf("a folder mid-write must yield no releases, got %+v", ev)
	}
}

// seededSeries builds a title folder holding one season of the given files.
func seededSeries(t *testing.T, files []string) (Deps, string) {
	t.Helper()
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":                {{Name: "Gentlemen 2024", Path: "/tv-show/Gentlemen 2024"}},
			"/tv-show/Gentlemen 2024": {{Name: "Season 01"}},
		},
		files: map[string][]string{"/tv-show/Gentlemen 2024/Season 01": files},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	return d, "Gentlemen 2024"
}

// Spec 1026 US1: the real source renames every file it serves with its own
// suffix, so four different encoders report one group and the token comparison
// separates nothing. The file each option PRODUCES still separates them.
func TestOptionIsMarkedByTheFileItProduces(t *testing.T) {
	// One quality of one season is on the NAS.
	d, title := seededSeries(t, []string{
		"The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv",
		"The.Gentlemen.S01E02.720p.WEB-DL.x264.Pahe-ZarFilm.mkv",
	})
	_, _, ev := d.titleDetail(context.Background(), title, library.MediaTV, nil)

	// Every option carries the site's own name as its group, so tokens cannot
	// tell any of these apart — only the files can.
	got := markOwnedOptions([]source.QualityOption{
		{ID: "nhtfs", Season: "Season 1", Resolution: "1080p", Encoder: "ZarFilm",
			ReleaseName: "The.Gentlemen.S01E01.1080p.WEB-DL.DDP5.1.Atmos.H.264.NHTFS-ZarFilm.mkv"},
		{ID: "psa", Season: "Season 1", Resolution: "1080p", Encoder: "ZarFilm",
			ReleaseName: "The.Gentlemen.S01E01.1080p.10bit.WEB-DL.6CH.x265.PSA-ZarFilm.mkv"},
		{ID: "pahe", Season: "Season 1", Resolution: "720p", Encoder: "ZarFilm",
			ReleaseName: "The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv"},
		{ID: "rmt", Season: "Season 1", Resolution: "480p", Encoder: "ZarFilm",
			ReleaseName: "The.Gentlemen.S01E01.480p.WEB.x264.RMT-ZarFilm.mkv"},
	}, ev, nil)

	owned := map[string]bool{}
	for _, q := range got {
		owned[q.ID] = q.Owned
	}
	if !owned["pahe"] {
		t.Error("the option that produced the files on disk must be marked")
	}
	for _, id := range []string{"nhtfs", "psa", "rmt"} {
		if owned[id] {
			t.Errorf("%s did not produce these files and must not be marked", id)
		}
	}
}

// FR-004, the load-bearing rule: a known-file mismatch is FINAL. If it fell back
// to tokens here, every option would match — they all carry the same group — and
// the over-marking this replaces would be back.
func TestAKnownFileMismatchNeverFallsBackToTokens(t *testing.T) {
	d, title := seededSeries(t, []string{"The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv"})
	_, _, ev := d.titleDetail(context.Background(), title, library.MediaTV, nil)

	// This option's tokens WOULD match what is on disk (720p, group "zarfilm"),
	// but it is not the file that is there.
	got := markOwnedOptions([]source.QualityOption{
		{ID: "other", Season: "Season 1", Resolution: "720p", Encoder: "ZarFilm",
			ReleaseName: "The.Gentlemen.S01E01.720p.WEB-DL.x265.Someone-ZarFilm.mkv"},
	}, ev, nil)
	if got[0].Owned {
		t.Fatal("a file mismatch must be final; falling back to tokens re-marks everything")
	}
}

// Acceptance 3: the option names episode 1, the user has episodes 5 and 6. Same
// release, so it must still be recognised.
func TestAPartlyDownloadedSeasonStillIdentifiesItsRelease(t *testing.T) {
	d, title := seededSeries(t, []string{
		"The.Gentlemen.S01E05.720p.WEB-DL.x264.Pahe-ZarFilm.mkv",
		"The.Gentlemen.S01E06.720p.WEB-DL.x264.Pahe-ZarFilm.mkv",
	})
	_, _, ev := d.titleDetail(context.Background(), title, library.MediaTV, nil)

	got := markOwnedOptions([]source.QualityOption{
		{ID: "pahe", Season: "Season 1", ReleaseName: "The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv"},
	}, ev, nil)
	if !got[0].Owned {
		t.Fatal("a partial season identifies its release just as well as a full one")
	}
}

// Acceptance 4: something is here, but nothing this source offers produced it.
func TestAFileNoOptionProducedMarksNothing(t *testing.T) {
	d, title := seededSeries(t, []string{"Gentlemen.S01E01.BDRip.SomeoneElse.mkv"})
	own, seasons, ev := d.titleDetail(context.Background(), title, library.MediaTV, nil)
	if own != source.OwnershipOwned || len(seasons) == 0 {
		t.Fatalf("the season is still here: own=%q seasons=%+v", own, seasons)
	}
	got := markOwnedOptions([]source.QualityOption{
		{ID: "pahe", Season: "Season 1", ReleaseName: "The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv"},
	}, ev, nil)
	if got[0].Owned {
		t.Fatal("nothing here was produced by this option")
	}
}

// FR-005: a source that does not name its files keeps the token comparison, so
// nothing about spec 1025's behaviour shifts where it already worked.
func TestSourcesThatNameNoFileStillUseTokens(t *testing.T) {
	d, title := seededSeries(t, []string{"Show.S01E01.1080p.BluRay.x265-Silence.mkv"})
	_, _, ev := d.titleDetail(context.Background(), title, library.MediaTV, nil)

	got := markOwnedOptions([]source.QualityOption{
		{ID: "match", Season: "Season 1", Resolution: "1080p", Encoder: "Silence"},
		{ID: "miss", Season: "Season 1", Resolution: "1080p", Encoder: "TENEIGHTY"},
	}, ev, nil)
	if !got[0].Owned {
		t.Error("the token path must still mark a genuine match")
	}
	if got[1].Owned {
		t.Error("the token path must still reject a different encoder")
	}
}

// Spec 1028: the snapshot used to be built on the REQUEST context. A client that
// hung up mid-response cancelled the NAS listing, and the empty result was then
// cached for the full TTL — so one phone giving up blanked every ownership marker
// for every user for five minutes. A slow source makes clients hang up, so the
// day a source went down, the markers went with it.
func TestLibraryIndexSurvivesTheCallerHangingUp(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	// The caller is already gone by the time the snapshot is wanted.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ix := d.libraryIndex(ctx)
	if ix.IsEmpty() {
		t.Fatal("a dead caller must not stop a shared snapshot from being built")
	}
	if _, ok := ix.Lookup("Dune 2021", library.MediaMovie); !ok {
		t.Fatal("the snapshot should hold what the NAS actually reported")
	}
}

// A failed reading must not be held as "you own nothing" for the whole TTL.
func TestAFailedLibraryReadingIsRetriedPromptly(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid", err: errors.New("nas unreachable")}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	if !d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("an unreachable NAS yields nothing")
	}
	// The NAS comes back.
	fake.err = nil
	fake.subfolders = map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}}

	// Age the cache by less than the success TTL but more than the retry window.
	d.lib.mu.Lock()
	d.lib.builtAt = time.Now().Add(-time.Minute)
	d.lib.mu.Unlock()

	if d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("a failure must be retried well before the success TTL, or one blip costs five minutes of markers")
	}
}

// A SUCCESSFUL reading still gets the full TTL — the retry window must not turn
// into a NAS listing on every search.
func TestASuccessfulLibraryReadingKeepsTheFullTTL(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	d.libraryIndex(context.Background())
	before := fake.fileListCalls
	d.lib.mu.Lock()
	d.lib.builtAt = time.Now().Add(-time.Minute) // inside the success TTL
	d.lib.mu.Unlock()

	d.libraryIndex(context.Background())
	if fake.fileListCalls != before {
		t.Fatal("a good snapshot must be reused for the full TTL, not rebuilt every minute")
	}
}

// Spec 0010: a library renamed for a media server keeps NO release information in
// its file names — no resolution, no group — so reading them back identifies
// nothing, and the sheet could say a season was on the NAS without saying which
// version. What we sent, we know, and that survives any renaming.
func TestTheVersionWeSentIsMarked(t *testing.T) {
	sent := []store.SourceDownload{{
		Destination: "tv-show/Show 2020/Season 01", CatalogID: "1:show",
		QualityID: "s1-0", QualityLabel: "WEB-DL 1080p", QualityResolution: "1080p", QualityEncoder: "Alpha",
	}}
	got := markOwnedOptions([]source.QualityOption{
		{ID: "s1-0", Season: "Season 1", Label: "WEB-DL 1080p", Resolution: "1080p", Encoder: "Alpha"},
		{ID: "s1-1", Season: "Season 1", Label: "WEB-DL 1080p", Resolution: "1080p", Encoder: "Beta"},
		{ID: "s2-0", Season: "Season 2", Label: "WEB-DL 1080p", Resolution: "1080p", Encoder: "Alpha"},
	}, evidenceRec{}, sent)

	if !got[0].Owned {
		t.Error("the version we sent must be marked")
	}
	if got[1].Owned {
		t.Error("a different version of the same season must not be marked")
	}
	if got[2].Owned {
		t.Error("the same version of a DIFFERENT season must not be marked")
	}
}

// Ids are positional on some sources, so a listing that reorders would point at
// the wrong row. Agreeing on every describing field survives that.
func TestASentVersionIsFoundEvenIfItsIdMoved(t *testing.T) {
	sent := []store.SourceDownload{{
		Destination: "movie/Dune 2021", CatalogID: "1:dune",
		QualityID: "3", QualityLabel: "2160p · SoftSub", QualityResolution: "2160p", QualityEncoder: "FraMeSToR",
	}}
	got := markOwnedOptions([]source.QualityOption{
		{ID: "0", Label: "2160p · SoftSub", Resolution: "2160p", Encoder: "FraMeSToR"},
		{ID: "3", Label: "1080p · Dubbed", Resolution: "1080p", Encoder: "Someone"},
	}, evidenceRec{}, sent)

	if !got[0].Owned {
		t.Error("the option that describes itself the same way must be marked")
	}
	if got[1].Owned {
		t.Error("a different option must not be marked just because it reused the id")
	}
}

// A movie: the record names the title folder, and an option with no season is
// season 0 on both sides, so they line up without a special case.
func TestASentMovieVersionIsMarked(t *testing.T) {
	sent := []store.SourceDownload{{
		Destination: "movie/Dune 2021", CatalogID: "1:dune",
		QualityID: "a", QualityLabel: "2160p", QualityResolution: "2160p", QualityEncoder: "FraMeSToR",
	}}
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Label: "2160p", Resolution: "2160p", Encoder: "FraMeSToR"},
		{ID: "b", Label: "1080p", Resolution: "1080p", Encoder: "FraMeSToR"},
	}, evidenceRec{}, sent)
	if !got[0].Owned || got[1].Owned {
		t.Fatalf("wrong option marked: %+v", got)
	}
}

// Nothing sent, nothing readable from the files: nothing is claimed. That is the
// state a renamed library is in, and saying nothing is the honest answer.
func TestWithNoHistoryAndNoReadableNamesNothingIsMarked(t *testing.T) {
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Season: "Season 1", Label: "WEB-DL 1080p", Resolution: "1080p", Encoder: "Alpha"},
	}, evidenceRec{}, nil)
	if got[0].Owned {
		t.Fatal("nothing identifies this option; it must not be marked")
	}
}

// Spec 1028: recorded downloads are matched by WHERE they went, not by which
// catalog entry they came from.
//
// A catalog id names one source's listing, so the same film downloaded from one
// source went unrecognised while browsing the other — and ids recorded before
// sources could be told apart carry no source at all, which was 47 of 65 records
// on a real instance. The folder is the same folder either way.
func TestRecordedDownloadsAreMatchedByFolder(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	// Recorded with an id from a DIFFERENT source, and one with no source at all.
	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination: "movie/Dune 2021", CatalogID: "2:dune-from-the-other-source",
		QualityLabel: "2160p", QualityResolution: "2160p", QualityEncoder: "FraMeSToR",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination: "tv-show/Dark 2017/Season 01", CatalogID: "",
		QualityLabel: "1080p", QualityResolution: "1080p", QualityEncoder: "Silence",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	if got := d.versionsSentFor("movie/Dune 2021"); len(got) != 1 {
		t.Fatalf("a download from another source's listing must still be found: %d", len(got))
	}
	// A series' seasons live under the title folder and all belong to it.
	if got := d.versionsSentFor("tv-show/Dark 2017"); len(got) != 1 {
		t.Fatalf("a season's download must be found under its title folder: %d", len(got))
	}
	// A different title must not pick them up, including one that merely shares a
	// prefix.
	for _, other := range []string{"movie/Dune 2021 Part Two", "movie/Arrival", "tv-show/Severance"} {
		if got := d.versionsSentFor(other); len(got) != 0 {
			t.Fatalf("%q must match nothing, got %d", other, len(got))
		}
	}
}

// Spec 2015: the folder a download was sent to and the folder it now lives in
// are routinely not the same string — a media server renames it after the
// download lands, most often by appending the release year. Comparing them as
// exact text meant that on the reporting instance not one send record matched
// its own download.
func TestASentDownloadIsFoundAfterItsFolderIsRenamed(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination:  "movie/The Sheep Detectives",
		QualityLabel: "1080p · SoftSub · Pahe", QualityResolution: "1080p", QualityEncoder: "Pahe",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination:  "tv-show/Friends/Season 01",
		QualityLabel: "1080p", QualityResolution: "1080p", QualityEncoder: "Silence",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}

	// The year the media server added must not lose the record.
	if got := d.versionsSentFor("movie/The Sheep Detectives (2026)"); len(got) != 1 {
		t.Fatalf("a renamed movie folder must still find its send: %d", len(got))
	}
	// And a season under a renamed series folder still belongs to it.
	got := d.versionsSentFor("tv-show/Friends (1994)")
	if len(got) != 1 {
		t.Fatalf("a renamed series folder must still find its season's send: %d", len(got))
	}
	if seasonOfDestination(got[0].Destination) != 1 {
		t.Errorf("the season must survive the match, got %d", seasonOfDestination(got[0].Destination))
	}

	// The conservative half. A record with no year matches a folder that has
	// gained one — that IS the rename, and it is the same rule the index used to
	// decide this folder was the title in the first place. What must not match is
	// a different name, or a year that positively disagrees.
	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination:  "movie/Arrival 2016",
		QualityLabel: "1080p", QualityResolution: "1080p", QualityEncoder: "Silence",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	for _, other := range []string{
		"movie/Arrival (2021)",             // both say a year, and they disagree
		"movie/The Sheep Detective (2026)", // a different name
	} {
		if got := d.versionsSentFor(other); len(got) != 0 {
			t.Errorf("%q must match nothing, got %d", other, len(got))
		}
	}
}

// A title folder in the WRONG parent is a different title. Matching on the name
// alone would let a film and a series of the same name claim each other's sends.
func TestASentDownloadIsNotFoundUnderAnotherParent(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	if err := st.SaveSourceDownload(store.SourceDownload{
		Destination:  "movie/Fargo",
		QualityLabel: "1080p", QualityResolution: "1080p", QualityEncoder: "Silence",
	}, 1); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := d.versionsSentFor("tv-show/Fargo (2014)"); len(got) != 0 {
		t.Fatalf("the film's send must not answer for the series, got %d", len(got))
	}
}

// --- spec 0011: the reading survives a restart and a blip -------------------

// A restart used to throw the whole reading away, so the markers were gone until
// somebody browsed and paid for a fresh NAS read. Deploys are frequent; this was
// the failure the user actually noticed.
func TestLibraryIndexIsServedFromTheStoreOnAColdCache(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	// Warm it once, which is what persists the reading.
	if d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("precondition: the first reading should succeed")
	}

	// Restart: a brand-new cache over the SAME store, and a NAS that now answers
	// nothing at all, so anything found can only have come from the store.
	restarted := d
	restarted.lib = &libraryCache{}
	fake.err = errors.New("nas unreachable")

	ix := restarted.libraryIndex(context.Background())
	if ix.IsEmpty() {
		t.Fatal("a restart must not lose the reading: it is stored")
	}
	if _, ok := ix.Lookup("Dune 2021", library.MediaMovie); !ok {
		t.Fatal("the stored reading should hold what the last good scan found")
	}
}

// A blip used to cache an EMPTY reading, which reads to a user as "you own
// nothing" and blanked every marker at once.
func TestAFailedLibraryReadingFallsBackToTheStoredOne(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	if d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("precondition: the first reading should succeed")
	}

	// The NAS goes away, and the cache ages past the retry window.
	fake.err = errors.New("nas unreachable")
	d.lib.mu.Lock()
	d.lib.builtAt = time.Now().Add(-time.Hour)
	d.lib.mu.Unlock()

	ix := d.libraryIndex(context.Background())
	if ix.IsEmpty() {
		t.Fatal("an unreachable NAS must fall back to the last good reading, not blank ownership")
	}
	if _, ok := ix.Lookup("Dune 2021", library.MediaMovie); !ok {
		t.Fatal("the fallback should still match what the last good scan found")
	}
}

// The fallback must not outlive the configuration it describes: a source removed
// while the NAS is down must not keep answering out of the store.
func TestTheStoredReadingIsClearedWhenNoParentIsConfigured(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	if d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("precondition: the first reading should succeed")
	}

	providers, err := st.ListProviders()
	if err != nil || len(providers) != 1 {
		t.Fatalf("ListProviders: %v (%d)", err, len(providers))
	}
	if err := st.DeleteProvider(providers[0].ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	d.invalidateLibrary()

	if !d.libraryIndex(context.Background()).IsEmpty() {
		t.Fatal("with nothing configured the reading must be empty, stored or not")
	}
}

// --- spec 0011: a title folder's reading is kept too ------------------------

// Opening a title used to cost a NAS read per folder on the first look, and the
// answer was thrown away on restart. Keeping it means the reading is there
// immediately, and a NAS that has gone away does not turn "you have season 1"
// into "we cannot say".
func TestFolderEvidenceIsServedFromTheStoreOnAColdCache(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
		},
		files: map[string][]string{
			"/movie/Dune 2021": {"Dune.2021.1080p.BluRay.x264-Silence.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	rec, ok := d.folderEvidence(context.Background(), "movie/Dune 2021")
	if !ok || !rec.hasVideo {
		t.Fatalf("precondition: the first reading should find video (ok=%v)", ok)
	}
	before := fake.fileListCalls

	// Restart: a fresh cache over the same store, and a NAS that answers nothing.
	restarted := d
	restarted.lib = &libraryCache{}
	fake.err = errors.New("nas unreachable")

	rec2, ok2 := restarted.folderEvidence(context.Background(), "movie/Dune 2021")
	if !ok2 {
		t.Fatal("a restart must not lose a folder's reading: it is stored")
	}
	if !rec2.hasVideo {
		t.Error("the stored reading should still say the folder holds video")
	}
	if fake.fileListCalls != before {
		t.Errorf("the stored reading must not cost a NAS read: %d extra", fake.fileListCalls-before)
	}
}

// The release tokens are what mark WHICH version the user has, so they have to
// survive the round-trip or the marker degrades to nothing after a restart.
func TestStoredFolderEvidenceKeepsSeasonsAndReleases(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/tv-show":           {{Name: "Dark 2017", Path: "/tv-show/Dark 2017"}},
			"/tv-show/Dark 2017": {{Name: "Season 01", Path: "/tv-show/Dark 2017/Season 01"}},
		},
		files: map[string][]string{
			"/tv-show/Dark 2017/Season 01": {
				"Dark.S01E01.1080p.WEB-DL.x265-Silence.mkv",
				"Dark.S01E02.1080p.WEB-DL.x265-Silence.mkv",
			},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	want, ok := d.folderEvidence(context.Background(), "tv-show/Dark 2017")
	if !ok || len(want.seasons) == 0 {
		t.Fatalf("precondition: seasons should be read (ok=%v, %d)", ok, len(want.seasons))
	}

	restarted := d
	restarted.lib = &libraryCache{}
	fake.err = errors.New("nas unreachable")

	got, ok := restarted.folderEvidence(context.Background(), "tv-show/Dark 2017")
	if !ok {
		t.Fatal("the stored reading should answer")
	}
	if len(got.seasons) != len(want.seasons) {
		t.Fatalf("seasons = %d, want %d", len(got.seasons), len(want.seasons))
	}
	if got.seasons[0].Season != want.seasons[0].Season ||
		len(got.seasons[0].Episodes) != len(want.seasons[0].Episodes) {
		t.Errorf("season detail did not survive: %+v want %+v", got.seasons[0], want.seasons[0])
	}
	if len(got.releases[1]) != len(want.releases[1]) {
		t.Errorf("releases = %v, want %v", got.releases[1], want.releases[1])
	}
	if len(got.keys[1]) != len(want.keys[1]) {
		t.Errorf("file keys = %v, want %v", got.keys[1], want.keys[1])
	}
}

// A folder that has never been read must still reach the NAS — the store is a
// fallback and a head start, not a substitute for ever looking.
func TestAnUnknownFolderIsStillReadFromTheNAS(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		files:    map[string][]string{"/movie/Arrival": {"Arrival.2016.1080p.BluRay.x264-Silence.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	rec, ok := d.folderEvidence(context.Background(), "movie/Arrival")
	if !ok || !rec.hasVideo {
		t.Fatalf("a folder nobody has read must be read (ok=%v)", ok)
	}
	if fake.fileListCalls == 0 {
		t.Error("it should have cost a NAS read")
	}
}

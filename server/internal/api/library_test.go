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

	_, _, releases := d.titleDetail(context.Background(), "Show 2020", library.MediaTV, nil)
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Season: "Season 1", Resolution: "1080p", Encoder: "Silence"},
		{ID: "b", Season: "Season 1", Resolution: "1080p", Encoder: "TENEIGHTY"},
		{ID: "c", Season: "Season 1", Resolution: "720p", Encoder: "Silence"},
		{ID: "d", Season: "Season 2", Resolution: "1080p", Encoder: "Silence"},
	}, releases)

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

	own, seasons, releases := d.titleDetail(context.Background(), "Show 2020", library.MediaTV, nil)
	if own != source.OwnershipOwned {
		t.Fatalf("ownership = %q, want owned — an unidentifiable release is still a release", own)
	}
	if len(seasons) != 1 || len(seasons[0].Episodes) != 2 {
		t.Fatalf("season presence must be unaffected: %+v", seasons)
	}
	for _, q := range markOwnedOptions([]source.QualityOption{
		{ID: "a", Season: "Season 1", Resolution: "1080p", Encoder: "Silence"},
	}, releases) {
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

	_, _, releases := d.titleDetail(context.Background(), "Dune 2021", library.MediaMovie, nil)
	got := markOwnedOptions([]source.QualityOption{
		{ID: "a", Resolution: "2160p", Encoder: "FraMeSToR"},
		{ID: "b", Resolution: "1080p", Encoder: "FraMeSToR"},
	}, releases)
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

	own, _, releases := d.titleDetail(context.Background(), "Dune 2021", library.MediaMovie,
		map[string]bool{"movie/Dune 2021": true})
	if own != source.OwnershipDownloading {
		t.Fatalf("ownership = %q, want downloading", own)
	}
	if len(releases) != 0 {
		t.Fatalf("a folder mid-write must yield no releases, got %+v", releases)
	}
}

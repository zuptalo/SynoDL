package api

import (
	"context"
	"errors"
	"path/filepath"
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

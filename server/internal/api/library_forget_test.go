package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// forgetDeps wires a stack whose NAS holds `folders` with `files`, and seeds one
// recorded download per destination given.
func forgetDeps(t *testing.T, folders map[string][]syno.Folder, files map[string][]string, dests ...string) (Deps, *store.Store, *fakeSyno) {
	t.Helper()
	fake := &fakeSyno{loginSid: "sid", subfolders: folders, files: files}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")
	for _, dest := range dests {
		if err := st.SaveSourceDownload(store.SourceDownload{
			Destination: dest, Title: dest, QualityResolution: "1080p", QualityEncoder: "Pahe",
		}, 1); err != nil {
			t.Fatalf("seed %s: %v", dest, err)
		}
	}
	return d, st, fake
}

func missingSince(t *testing.T, st *store.Store, dest string) time.Time {
	t.Helper()
	all, err := st.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	rec, ok := all[dest]
	if !ok {
		t.Fatalf("no record for %q", dest)
	}
	return rec.MissingSince
}

func exists(t *testing.T, st *store.Store, dest string) bool {
	t.Helper()
	all, err := st.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	_, ok := all[dest]
	return ok
}

// THE dangerous case. A managed library renames a folder after the download
// lands, on essentially every title. If a rename read as a removal, one scan
// would delete the entire record set — and those records are the only thing
// that knows which version was downloaded.
func TestARenamedFolderIsNotAnAbsentOne(t *testing.T) {
	d, st, _ := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		map[string][]string{"/movie/Dune 2021": {"Dune.2021.1080p.BluRay.x264-Silence.mkv"}},
		"movie/Dune", // sent before the media server added the year
	)
	d.scanOnce(context.Background())

	if got := missingSince(t, st, "movie/Dune"); !got.IsZero() {
		t.Fatalf("a renamed folder was marked missing at %v; every record would be deleted", got)
	}
	if !exists(t, st, "movie/Dune") {
		t.Fatal("the record was deleted for a folder that is merely renamed")
	}
}

// A folder that no longer exists at all.
func TestAnAbsentFolderIsMarkedThenDeleted(t *testing.T) {
	d, st, _ := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Kept", Path: "/movie/Kept"}}},
		map[string][]string{"/movie/Kept": {"Kept.2020.1080p.BluRay.x264-Silence.mkv"}},
		"movie/Kept", "movie/Gone",
	)

	d.scanOnce(context.Background())
	first := missingSince(t, st, "movie/Gone")
	if first.IsZero() {
		t.Fatal("an absent folder should be marked on the first cycle that sees it gone")
	}
	if !exists(t, st, "movie/Gone") {
		t.Fatal("it must NOT be deleted on the cycle that first sees it gone")
	}
	if got := missingSince(t, st, "movie/Kept"); !got.IsZero() {
		t.Errorf("a folder that is present was marked missing at %v", got)
	}

	// Age the mark past the grace period; the next cycle deletes it.
	if err := st.MarkSourceDownloadMissing("movie/Gone", time.Now().Add(-forgetGrace-time.Minute)); err != nil {
		t.Fatalf("age the mark: %v", err)
	}
	if err := st.ClearSourceDownloadMissing("movie/Gone"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := st.MarkSourceDownloadMissing("movie/Gone", time.Now().Add(-forgetGrace-time.Minute)); err != nil {
		t.Fatalf("re-age: %v", err)
	}

	d.scanOnce(context.Background())
	if exists(t, st, "movie/Gone") {
		t.Error("a folder gone for the whole grace period should be forgotten")
	}
	if !exists(t, st, "movie/Kept") {
		t.Error("the folder that is still there was forgotten too")
	}
}

// The case the user actually described: the folder is still there, but the files
// have been deleted out of it.
func TestAFolderWithNoVideoIsTreatedAsGone(t *testing.T) {
	d, st, _ := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Emptied", Path: "/movie/Emptied"}}},
		// Left behind by a media server: metadata, no video.
		map[string][]string{"/movie/Emptied": {"poster.jpg", "movie.nfo"}},
		"movie/Emptied",
	)
	d.scanOnce(context.Background())
	if got := missingSince(t, st, "movie/Emptied"); got.IsZero() {
		t.Fatal("a folder holding no video should be treated as gone")
	}
	if !exists(t, st, "movie/Emptied") {
		t.Fatal("it must not be deleted on the first cycle")
	}
}

// Putting the content back before the grace period elapses must keep the record.
func TestAFolderThatComesBackClearsItsMark(t *testing.T) {
	d, st, fake := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Blip", Path: "/movie/Blip"}}},
		map[string][]string{"/movie/Blip": {"poster.jpg"}},
		"movie/Blip",
	)
	d.scanOnce(context.Background())
	if missingSince(t, st, "movie/Blip").IsZero() {
		t.Fatal("precondition: the empty folder should be marked")
	}

	// The file reappears (an extract finished, a move completed).
	fake.files["/movie/Blip"] = []string{"Blip.2020.1080p.BluRay.x264-Silence.mkv"}
	d.invalidateLibrary()
	d.scanOnce(context.Background())

	if got := missingSince(t, st, "movie/Blip"); !got.IsZero() {
		t.Errorf("the mark should be cleared once the content is back, got %v", got)
	}
	if !exists(t, st, "movie/Blip") {
		t.Error("the record was deleted for content that came back")
	}
}

// A NAS that cannot be read must change nothing at all. Treating an unreadable
// NAS as an empty one would delete every record on the first outage.
func TestAnUnreachableNASForgetsNothing(t *testing.T) {
	d, st, fake := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Kept", Path: "/movie/Kept"}}},
		map[string][]string{"/movie/Kept": {"Kept.2020.1080p.BluRay.x264-Silence.mkv"}},
		"movie/Kept",
	)
	d.scanOnce(context.Background()) // warm a good reading

	fake.err = errors.New("nas unreachable")
	d.invalidateLibrary()
	d.scanOnce(context.Background())

	if got := missingSince(t, st, "movie/Kept"); !got.IsZero() {
		t.Errorf("an unreachable NAS marked a record missing at %v", got)
	}
	if !exists(t, st, "movie/Kept") {
		t.Error("an unreachable NAS deleted a record")
	}
}

// A folder with an unfinished task holds no video yet, and is not gone.
//
// PAUSED is the case that matters and the one an "is it downloading" check gets
// wrong: a paused task is deliberately not "active" — Discover must not badge it
// as arriving — but its folder is empty for a perfectly good reason and will not
// be when it resumes. Cleaning it up would delete the record of what was being
// fetched, mid-fetch.
func TestAFolderWithAnUnfinishedTaskIsNotGone(t *testing.T) {
	d, st, _ := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {
			{Name: "Arriving", Path: "/movie/Arriving"},
			{Name: "Paused", Path: "/movie/Paused"},
		}},
		map[string][]string{
			"/movie/Arriving": {"Arriving.2024.1080p.mkv.part"},
			"/movie/Paused":   {},
		},
		"movie/Arriving", "movie/Paused",
	)
	// The watcher reports both as unfinished; only the first is "active".
	d.ActiveDests = func() map[string]bool { return map[string]bool{"movie/Arriving": true} }
	d.PendingDests = func() map[string]bool {
		return map[string]bool{"movie/Arriving": true, "movie/Paused": true}
	}

	d.scanOnce(context.Background())
	for _, dest := range []string{"movie/Arriving", "movie/Paused"} {
		if got := missingSince(t, st, dest); !got.IsZero() {
			t.Errorf("%s has an unfinished task but was marked missing at %v", dest, got)
		}
	}
}

// And a paused task that is then removed from Download Station stops protecting
// its folder, so genuinely abandoned content is still cleaned up.
func TestAFolderStopsBeingProtectedOnceItsTaskIsGone(t *testing.T) {
	d, st, _ := forgetDeps(t,
		map[string][]syno.Folder{"/movie": {{Name: "Abandoned", Path: "/movie/Abandoned"}}},
		map[string][]string{"/movie/Abandoned": {}},
		"movie/Abandoned",
	)
	d.PendingDests = func() map[string]bool { return map[string]bool{"movie/Abandoned": true} }
	d.scanOnce(context.Background())
	if got := missingSince(t, st, "movie/Abandoned"); !got.IsZero() {
		t.Fatalf("precondition: a pending task should protect the folder, got %v", got)
	}

	// The user deletes the task.
	d.PendingDests = func() map[string]bool { return nil }
	d.invalidateLibrary()
	d.scanOnce(context.Background())
	if missingSince(t, st, "movie/Abandoned").IsZero() {
		t.Error("with no task left, an empty folder should be marked gone")
	}
}

// An operator may configure a nested parent ("media/movie" rather than "movie"),
// which makes every destination one segment longer. The reconciliation counts
// segments to find the title folder, so this is where an off-by-one would hide —
// and the failure mode is marking real content as gone.
func TestNestedParentsResolveToTheRightTitleFolder(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/media/movie":        {{Name: "Season of the Witch", Path: "/media/movie/Season of the Witch"}},
			"/media/tv":           {{Name: "Dark 2017", Path: "/media/tv/Dark 2017"}},
			"/media/tv/Dark 2017": {{Name: "Season 01", Path: "/media/tv/Dark 2017/Season 01"}},
		},
		files: map[string][]string{
			// A film whose TITLE looks like a season folder. It must not be mistaken
			// for one and stripped away.
			"/media/movie/Season of the Witch": {"Season.of.the.Witch.2011.1080p.BluRay.x264-Silence.mkv"},
			"/media/tv/Dark 2017/Season 01":    {"Dark.S01E01.1080p.WEB-DL.x265-Silence.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	if _, err := st.CreateProvider(store.SourceProvider{
		Kind: "faketest", DisplayName: "Alpha", Enabled: true,
		MoviesParent: "media/movie", TVParent: "media/tv", State: store.SourceActive,
	}, time.Now().Unix()); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	for _, dest := range []string{"media/movie/Season of the Witch", "media/tv/Dark 2017/Season 01"} {
		if err := st.SaveSourceDownload(store.SourceDownload{
			Destination: dest, Title: dest, QualityResolution: "1080p", QualityEncoder: "Silence",
		}, 1); err != nil {
			t.Fatalf("seed %s: %v", dest, err)
		}
	}

	d.scanOnce(context.Background())

	for _, dest := range []string{"media/movie/Season of the Witch", "media/tv/Dark 2017/Season 01"} {
		if got := missingSince(t, st, dest); !got.IsZero() {
			t.Errorf("%q is present on the NAS but was marked gone at %v", dest, got)
		}
	}
}

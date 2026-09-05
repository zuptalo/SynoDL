package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// A scan refreshes the parent listings and keeps them, so a restart has
// something to start from without anyone having browsed first.
func TestAScanKeepsTheParentListing(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie":   {{Name: "Dune 2021", Path: "/movie/Dune 2021"}},
			"/tv-show": {{Name: "Dark 2017", Path: "/tv-show/Dark 2017"}},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "tv-show")

	d.scanOnce(context.Background())

	f, err := st.GetLibraryFolders()
	if err != nil {
		t.Fatalf("GetLibraryFolders: %v", err)
	}
	if len(f.Names["movie"]) != 1 || len(f.Names["tv-show"]) != 1 {
		t.Fatalf("the scan should have kept both parents, got %+v", f.Names)
	}
}

// A cycle must cost a bounded number of NAS reads however large the library is,
// or the scan becomes the thing that hammers the NAS it was meant to spare.
func TestAScanIsBoundedPerCycle(t *testing.T) {
	folders := make([]syno.Folder, 0, scanFolderBudget*3)
	files := map[string][]string{}
	for i := 0; i < scanFolderBudget*3; i++ {
		name := "Film " + string(rune('A'+i%26)) + string(rune('a'+i/26))
		folders = append(folders, syno.Folder{Name: name, Path: "/movie/" + name})
		files["/movie/"+name] = []string{name + ".2020.1080p.BluRay.x264-Silence.mkv"}
	}
	fake := &fakeSyno{loginSid: "sid", subfolders: map[string][]syno.Folder{"/movie": folders}, files: files}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	d.scanOnce(context.Background())

	read := 0
	for i := 0; i < len(folders); i++ {
		if _, found, _ := st.GetLibraryEvidence("movie/" + folders[i].Name); found {
			read++
		}
	}
	if read == 0 {
		t.Fatal("a cycle should read some folders")
	}
	if read > scanFolderBudget {
		t.Fatalf("a cycle read %d folders, budget is %d", read, scanFolderBudget)
	}
}

// Successive cycles must move through the library rather than re-reading the
// same folders, or the tail never gets read at all.
func TestSuccessiveScansAdvanceThroughTheLibrary(t *testing.T) {
	folders := make([]syno.Folder, 0, scanFolderBudget+5)
	files := map[string][]string{}
	for i := 0; i < scanFolderBudget+5; i++ {
		name := "Film " + string(rune('A'+i%26)) + string(rune('a'+i/26))
		folders = append(folders, syno.Folder{Name: name, Path: "/movie/" + name})
		files["/movie/"+name] = []string{name + ".2020.1080p.BluRay.x264-Silence.mkv"}
	}
	fake := &fakeSyno{loginSid: "sid", subfolders: map[string][]syno.Folder{"/movie": folders}, files: files}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	count := func() int {
		n := 0
		for _, f := range folders {
			if _, found, _ := st.GetLibraryEvidence("movie/" + f.Name); found {
				n++
			}
		}
		return n
	}
	d.scanOnce(context.Background())
	first := count()
	d.scanOnce(context.Background())
	if second := count(); second <= first {
		t.Fatalf("a second cycle must reach folders the first did not: %d then %d", first, second)
	}
}

// A folder a download just landed in must not wait its turn behind a whole
// library — that is the case the user is actually watching.
func TestAnEnqueuedFolderJumpsTheQueue(t *testing.T) {
	folders := make([]syno.Folder, 0, scanFolderBudget*2)
	files := map[string][]string{}
	for i := 0; i < scanFolderBudget*2; i++ {
		name := "Film " + string(rune('A'+i%26)) + string(rune('a'+i/26))
		folders = append(folders, syno.Folder{Name: name, Path: "/movie/" + name})
		files["/movie/"+name] = []string{name + ".2020.1080p.BluRay.x264-Silence.mkv"}
	}
	target := "Zebra Late"
	folders = append(folders, syno.Folder{Name: target, Path: "/movie/" + target})
	files["/movie/"+target] = []string{"Zebra.Late.2024.1080p.BluRay.x264-Silence.mkv"}

	fake := &fakeSyno{loginSid: "sid", subfolders: map[string][]syno.Folder{"/movie": folders}, files: files}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	d.RefreshFolder("movie/" + target)
	d.scanOnce(context.Background())

	if _, found, _ := st.GetLibraryEvidence("movie/" + target); !found {
		t.Fatal("a folder enqueued after a download must be read in the next cycle")
	}
}

// A title folder deleted on the NAS must stop being answered for.
func TestAScanForgetsAFolderThatIsGone(t *testing.T) {
	fake := &fakeSyno{
		loginSid: "sid",
		subfolders: map[string][]syno.Folder{
			"/movie": {{Name: "Kept", Path: "/movie/Kept"}, {Name: "Gone", Path: "/movie/Gone"}},
		},
		files: map[string][]string{
			"/movie/Kept": {"Kept.2020.1080p.BluRay.x264-Silence.mkv"},
			"/movie/Gone": {"Gone.2020.1080p.BluRay.x264-Silence.mkv"},
		},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	d.scanOnce(context.Background())
	if _, found, _ := st.GetLibraryEvidence("movie/Gone"); !found {
		t.Fatal("precondition: both folders should have been read")
	}

	// The folder is deleted on the NAS.
	fake.subfolders["/movie"] = []syno.Folder{{Name: "Kept", Path: "/movie/Kept"}}
	d.scanOnce(context.Background())

	if _, found, _ := st.GetLibraryEvidence("movie/Gone"); found {
		t.Error("a folder no longer on the NAS is still answered for")
	}
	if _, found, _ := st.GetLibraryEvidence("movie/Kept"); !found {
		t.Error("the scan forgot a folder that is still there")
	}
}

// A scan that cannot reach the NAS must change nothing. Treating a failed scan
// as "the NAS holds nothing" would delete the fallback the feature exists for.
func TestAFailedScanKeepsWhatIsAlreadyKnown(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Kept", Path: "/movie/Kept"}}},
		files:      map[string][]string{"/movie/Kept": {"Kept.2020.1080p.BluRay.x264-Silence.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")
	d.scanOnce(context.Background())

	fake.err = errors.New("nas unreachable")
	d.scanOnce(context.Background())

	f, err := st.GetLibraryFolders()
	if err != nil {
		t.Fatalf("GetLibraryFolders: %v", err)
	}
	if len(f.Names["movie"]) != 1 {
		t.Errorf("a failed scan wiped the stored listing: %+v", f.Names)
	}
	if _, found, _ := st.GetLibraryEvidence("movie/Kept"); !found {
		t.Error("a failed scan wiped the stored folder reading")
	}
}

// Nothing configured means nothing stored — a scan must not keep answering for
// a source the operator has removed.
func TestAScanWithNoSourceClearsTheStoredReading(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Kept", Path: "/movie/Kept"}}},
		files:      map[string][]string{"/movie/Kept": {"Kept.2020.1080p.BluRay.x264-Silence.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")
	d.scanOnce(context.Background())

	providers, _ := st.ListProviders()
	for _, p := range providers {
		if err := st.DeleteProvider(p.ID); err != nil {
			t.Fatalf("DeleteProvider: %v", err)
		}
	}
	d.scanOnce(context.Background())

	f, _ := st.GetLibraryFolders()
	if len(f.Names) != 0 {
		t.Errorf("a removed source still has folders stored: %+v", f.Names)
	}
}

var _ = time.Minute
var _ = store.LibraryFolders{}

// The scan is started from a Deps that never goes through NewRouter. Deps is a
// VALUE and NewRouter allocates the caches on its own copy, so the scan held a
// nil cache and did nothing — no error, no log line, a feature that simply never
// ran. It shipped that way until someone checked whether the table was filling.
func TestBackgroundWorkSharesTheRouterCache(t *testing.T) {
	fake := &fakeSyno{
		loginSid:   "sid",
		subfolders: map[string][]syno.Folder{"/movie": {{Name: "Dune 2021", Path: "/movie/Dune 2021"}}},
		files:      map[string][]string{"/movie/Dune 2021": {"Dune.2021.1080p.BluRay.x264-Silence.mkv"}},
	}
	d, st := libDeps(t, fake)
	addSource(t, st, "Alpha", "movie", "")

	// Exactly what main.go does: prepare Deps, keep it for background work, and
	// hand the SAME value to the router.
	prepared := InitCaches(Deps{Cfg: d.Cfg, Stateful: true, Store: d.Store, NAS: d.NAS})
	_ = NewRouter(prepared)

	if prepared.lib == nil {
		t.Fatal("InitCaches must leave the caches allocated on the value it returns")
	}
	prepared.scanOnce(context.Background())

	if _, found, _ := st.GetLibraryEvidence("movie/Dune 2021"); !found {
		t.Fatal("a scan started the way main.go starts it must actually read and keep something")
	}
	// And the enqueue path has to reach the same cache.
	prepared.RefreshFolder("movie/Dune 2021")
	prepared.lib.mu.Lock()
	queued := len(prepared.lib.scanQueue)
	prepared.lib.mu.Unlock()
	if queued != 1 {
		t.Fatalf("RefreshFolder reached no cache: %d queued", queued)
	}
}

// NewRouter must not replace a cache the caller already allocated, or the
// sharing InitCaches exists for is undone the moment the router is built.
func TestNewRouterKeepsACachePassedToIt(t *testing.T) {
	fake := &fakeSyno{loginSid: "sid"}
	d, _ := libDeps(t, fake)
	prepared := InitCaches(d)
	want := prepared.lib
	_ = NewRouter(prepared)
	if prepared.lib != want {
		t.Fatal("NewRouter replaced the caller's cache")
	}
}

package store

import (
	"reflect"
	"testing"
	"time"
)

func TestLibraryFoldersRoundTrip(t *testing.T) {
	s := openTestStore(t)
	at := time.Unix(1_700_000_000, 0)

	in := LibraryFolders{
		Parents: []LibraryParent{
			{Path: "movie", Movies: true},
			{Path: "tv-show", TV: true},
		},
		Names: map[string][]string{
			"movie":   {"Dune 2021", "Interstellar"},
			"tv-show": {"Friends"},
		},
		ScannedAt: at,
	}
	if err := s.SaveLibraryFolders(in); err != nil {
		t.Fatalf("SaveLibraryFolders: %v", err)
	}

	got, err := s.GetLibraryFolders()
	if err != nil {
		t.Fatalf("GetLibraryFolders: %v", err)
	}
	if !reflect.DeepEqual(got.Parents, in.Parents) {
		t.Errorf("parents = %+v, want %+v", got.Parents, in.Parents)
	}
	if !reflect.DeepEqual(got.Names, in.Names) {
		t.Errorf("names = %+v, want %+v", got.Names, in.Names)
	}
	if !got.ScannedAt.Equal(at) {
		t.Errorf("scannedAt = %v, want %v", got.ScannedAt, at)
	}
}

// A parent an operator has stopped configuring must leave nothing behind: the
// stored reading exists to answer for the CURRENT configuration, and a folder
// under a disconnected parent is not part of it (FR-009).
func TestLibraryFoldersReplaceDropsARemovedParent(t *testing.T) {
	s := openTestStore(t)
	at := time.Unix(1_700_000_000, 0)

	if err := s.SaveLibraryFolders(LibraryFolders{
		Parents:   []LibraryParent{{Path: "movie", Movies: true}, {Path: "old", TV: true}},
		Names:     map[string][]string{"movie": {"Dune 2021"}, "old": {"Leftover"}},
		ScannedAt: at,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.SaveLibraryFolders(LibraryFolders{
		Parents:   []LibraryParent{{Path: "movie", Movies: true}},
		Names:     map[string][]string{"movie": {"Dune 2021", "Arrival"}},
		ScannedAt: at.Add(time.Minute),
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := s.GetLibraryFolders()
	if err != nil {
		t.Fatalf("GetLibraryFolders: %v", err)
	}
	if _, still := got.Names["old"]; still {
		t.Error("a parent that is no longer configured is still stored")
	}
	if want := []string{"Arrival", "Dune 2021"}; !reflect.DeepEqual(got.Names["movie"], want) {
		t.Errorf("movie = %v, want %v", got.Names["movie"], want)
	}
}

// An empty scan must not be mistaken for "nothing was ever stored" by the
// caller, but it also must not silently keep the previous reading: an operator
// who removes every source has no configured parents, and the store must say so.
func TestLibraryFoldersReplaceWithNothingClearsEverything(t *testing.T) {
	s := openTestStore(t)
	if err := s.SaveLibraryFolders(LibraryFolders{
		Parents:   []LibraryParent{{Path: "movie", Movies: true}},
		Names:     map[string][]string{"movie": {"Dune 2021"}},
		ScannedAt: time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.SaveLibraryFolders(LibraryFolders{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := s.GetLibraryFolders()
	if err != nil {
		t.Fatalf("GetLibraryFolders: %v", err)
	}
	if len(got.Parents) != 0 || len(got.Names) != 0 {
		t.Errorf("cleared store still holds %+v", got)
	}
}

func TestLibraryEvidenceRoundTrip(t *testing.T) {
	s := openTestStore(t)
	at := time.Unix(1_700_000_500, 0)

	in := LibraryEvidence{
		Path:     "tv-show/Friends",
		HasVideo: true,
		Seasons: []SeasonPresence{
			{Season: 1, Episodes: []int{1, 2, 3}, VideoFiles: 3},
			{Season: 2, Episodes: nil, VideoFiles: 2},
		},
		Releases: map[int][]ReleaseToken{
			1: {{Resolution: "1080p", Group: "ntb", Key: "friends.s01e01.1080p.ntb"}},
		},
		FileKeys:  map[int][]string{1: {"friends.s01e01.1080p.ntb"}},
		CheckedAt: at,
	}
	if err := s.SaveLibraryEvidence(in); err != nil {
		t.Fatalf("SaveLibraryEvidence: %v", err)
	}

	got, found, err := s.GetLibraryEvidence("tv-show/Friends")
	if err != nil || !found {
		t.Fatalf("GetLibraryEvidence: found=%v err=%v", found, err)
	}
	if !got.CheckedAt.Equal(at) {
		t.Errorf("checkedAt = %v, want %v", got.CheckedAt, at)
	}
	got.CheckedAt = in.CheckedAt
	if !reflect.DeepEqual(got, in) {
		t.Errorf("evidence = %+v, want %+v", got, in)
	}
}

func TestLibraryEvidenceMissingIsNotAnError(t *testing.T) {
	s := openTestStore(t)
	_, found, err := s.GetLibraryEvidence("movie/Never Seen")
	if err != nil {
		t.Fatalf("GetLibraryEvidence: %v", err)
	}
	if found {
		t.Error("found a reading for a folder that was never stored")
	}
}

// The scan refreshes the least-recently-read folders first, so a large library
// converges instead of re-reading the same few every cycle.
func TestStaleLibraryEvidenceIsOldestFirst(t *testing.T) {
	s := openTestStore(t)
	base := time.Unix(1_700_000_000, 0)
	for i, path := range []string{"movie/C", "movie/A", "movie/B"} {
		if err := s.SaveLibraryEvidence(LibraryEvidence{
			Path: path, HasVideo: true, CheckedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}
	got, err := s.StaleLibraryEvidence(2)
	if err != nil {
		t.Fatalf("StaleLibraryEvidence: %v", err)
	}
	if want := []string{"movie/C", "movie/A"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stale = %v, want %v (oldest first, limited)", got, want)
	}
}

// A title folder deleted on the NAS must stop being answered for. Keeping it
// would report content the user no longer has, which is the one direction this
// feature must never get wrong.
func TestPruneLibraryEvidenceDropsAVanishedFolder(t *testing.T) {
	s := openTestStore(t)
	at := time.Unix(1_700_000_000, 0)
	for _, path := range []string{"movie/Kept", "movie/Gone"} {
		if err := s.SaveLibraryEvidence(LibraryEvidence{Path: path, HasVideo: true, CheckedAt: at}); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}
	if err := s.PruneLibraryEvidence([]string{"movie/Kept"}); err != nil {
		t.Fatalf("PruneLibraryEvidence: %v", err)
	}
	if _, found, _ := s.GetLibraryEvidence("movie/Gone"); found {
		t.Error("a folder no longer on the NAS is still answered for")
	}
	if _, found, _ := s.GetLibraryEvidence("movie/Kept"); !found {
		t.Error("prune removed a folder that is still there")
	}
}

// Pruning against an empty set is the "we could not read anything" case. It must
// NOT be read as "the NAS holds nothing" — that would wipe the fallback the
// whole feature depends on.
func TestPruneLibraryEvidenceWithNoFoldersKeepsEverything(t *testing.T) {
	s := openTestStore(t)
	at := time.Unix(1_700_000_000, 0)
	if err := s.SaveLibraryEvidence(LibraryEvidence{Path: "movie/Kept", HasVideo: true, CheckedAt: at}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.PruneLibraryEvidence(nil); err != nil {
		t.Fatalf("PruneLibraryEvidence: %v", err)
	}
	if _, found, _ := s.GetLibraryEvidence("movie/Kept"); !found {
		t.Error("pruning against an empty folder set wiped the stored reading")
	}
}

package library

import "testing"

func TestReleaseOfReadsResolutionAndGroup(t *testing.T) {
	for _, tc := range []struct{ name, res, group string }{
		{"Show.S01E01.1080p.BluRay.x265-Silence.mkv", "1080p", "silence"},
		{"Show.S01E02.720p.WEB-DL.x264-TENEIGHTY.mkv", "720p", "teneighty"},
		{"Some Movie (2024) 2160p UHD BluRay-FraMeSToR.mkv", "2160p", "framestor"},
		{"Some Movie 480p DVDRip [YIFY].mp4", "480p", "yify"},
		// 4K and UHD are the same thing written differently; both normalise onto
		// the token an option would carry.
		{"Movie.4K.BluRay-PSA.mkv", "2160p", "psa"},
		{"Movie.UHD.WEB-DL-PSA.mkv", "2160p", "psa"},
	} {
		got, ok := ReleaseOf(tc.name)
		if !ok {
			t.Fatalf("%s: not identified", tc.name)
		}
		if got.Resolution != tc.res || got.Group != tc.group {
			t.Fatalf("%s: got %+v, want %s/%s", tc.name, got, tc.res, tc.group)
		}
	}
}

// FR-002: half an identification is no identification. Marking on a resolution
// alone is exactly the wrong guess — several releases of a season share one
// resolution and differ only by who encoded them.
func TestReleaseOfRefusesAPartialIdentification(t *testing.T) {
	for _, name := range []string{
		"Show.S01E01.1080p.BluRay.mkv", // resolution, no group
		"Show.S01E01-Silence.mkv",      // group, no resolution
		"Show - 1x01.mkv",
		"episode one.mkv",
		"",
		".mkv",
		"----.mkv",
		"Show.S01E01.1080p.BluRay-.mkv", // trailing dash with nothing after it
	} {
		if got, ok := ReleaseOf(name); ok {
			t.Fatalf("%q should not identify a release, got %+v", name, got)
		}
	}
}

// A group is compared without regard to case or separators, so the file's
// "x265-Silence" and an option's "Silence" agree.
func TestReleaseGroupIsNormalised(t *testing.T) {
	a, ok := ReleaseOf("Show.S01E01.1080p.BluRay.x265-SiLeNcE.mkv")
	if !ok {
		t.Fatal("not identified")
	}
	if a.Group != "silence" {
		t.Fatalf("group = %q, want it folded to a comparable form", a.Group)
	}
	b, _ := ReleaseOf("Show.S01E02.1080p.BluRay.x265-Silence.mkv")
	if a.Group != b.Group {
		t.Fatalf("%q and %q should agree", a.Group, b.Group)
	}
}

func TestReleaseMatchesOption(t *testing.T) {
	on := Release{Resolution: "1080p", Group: "silence"}
	if !on.Matches("1080p", "x265-Silence") {
		t.Fatal("the same release written differently must match")
	}
	if !on.Matches("1080p", "Silence") {
		t.Fatal("a bare group name must match")
	}
	// The rejected shortcut: same resolution, different encoder.
	if on.Matches("1080p", "TENEIGHTY") {
		t.Fatal("a resolution match alone must never be enough")
	}
	if on.Matches("720p", "Silence") {
		t.Fatal("a group match alone must never be enough")
	}
	if on.Matches("1080p", "") {
		t.Fatal("an option with no group cannot be identified")
	}
}

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

// The six qualities of ONE season of one real title, as the source actually
// serves them (captured 2026-09-04). The release group cannot tell these apart —
// five of them carry the site's own suffix and the sixth carries no separator at
// all — which is why identity is taken from the file itself (spec 1026).
var realZarfilmSeason = []string{
	"The.Gentlemen.S01E01.1080p.WEB-DL.DDP5.1.Atmos.H.264.NHTFS-ZarFilm.mkv",
	"The.Gentlemen.S01E01.Refined.Aggression.1080p.-ZarFilm.mkv",
	"The.Gentlemen.S01E01.1080p.10bit.WEB-DL.6CH.x265.PSA-ZarFilm.mkv",
	"The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv",
	"The.Gentlemen.S01E01.480p.WEB.x264.RMT-ZarFilm.mkv",
	"The.Gentlemen.S01E01.1080p.WEB-DL.Dubbed.ZarFilm.mkv",
}

func TestReleaseKeyTellsRealReleasesApart(t *testing.T) {
	seen := map[string]string{}
	for _, name := range realZarfilmSeason {
		k := ReleaseKey(name)
		if k == "" {
			t.Fatalf("%s: no key", name)
		}
		if prev, dup := seen[k]; dup {
			t.Fatalf("%s and %s share a key %q — these are different releases", prev, name, k)
		}
		seen[k] = name
	}
}

// A season the user only partly downloaded must still identify its release: the
// option names episode 1, and what is on disk may be episode 5.
func TestReleaseKeyIgnoresTheEpisodeNumber(t *testing.T) {
	first := ReleaseKey("The.Gentlemen.S01E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv")
	fifth := ReleaseKey("The.Gentlemen.S01E05.720p.WEB-DL.x264.Pahe-ZarFilm.mkv")
	if first == "" || first != fifth {
		t.Fatalf("episodes of one release must key alike: %q vs %q", first, fifth)
	}
	// The SEASON still matters — it is kept so two seasons of one release do not
	// collapse onto each other.
	if s2 := ReleaseKey("The.Gentlemen.S02E01.720p.WEB-DL.x264.Pahe-ZarFilm.mkv"); s2 == first {
		t.Fatal("different seasons must not key alike")
	}
	// The other common episode notation reduces the same way.
	if ReleaseKey("Show.1x01.720p.x264-Pahe.mkv") != ReleaseKey("Show.1x07.720p.x264-Pahe.mkv") {
		t.Fatal("1xNN episodes of one release must key alike")
	}
}

// Two movie options at one resolution, differing only by an encode no token can
// see. This is the pair the previous matcher could not separate.
func TestReleaseKeySeparatesEncodesAtTheSameResolution(t *testing.T) {
	dubbed := ReleaseKey("Coyote.vs.Acme.2026.1080p.WEBRip.Dubbed.ZarFilm.mkv")
	plain := ReleaseKey("Coyote.vs.Acme.2026.1080p.WEBRip.x264.ZarFilm.mkv")
	if dubbed == "" || dubbed == plain {
		t.Fatalf("these are different releases: %q vs %q", dubbed, plain)
	}
}

func TestReleaseKeyIsInsensitiveToFormatting(t *testing.T) {
	a := ReleaseKey("Some.Movie.2024.1080p.BluRay-Group.mkv")
	for _, variant := range []string{
		"some movie 2024 1080p bluray group.mkv",
		"Some_Movie_2024_1080p_BluRay_Group.mp4",
		"  Some.Movie.2024.1080p.BluRay-Group.mkv  ",
		"/volume1/movie/Some Movie/Some.Movie.2024.1080p.BluRay-Group.mkv",
	} {
		if got := ReleaseKey(variant); got != a {
			t.Fatalf("%q keyed %q, want %q", variant, got, a)
		}
	}
}

func TestReleaseKeyOfNothing(t *testing.T) {
	for _, name := range []string{"", "   ", ".mkv", "...", "/"} {
		if got := ReleaseKey(name); got != "" {
			t.Fatalf("%q keyed %q, want empty", name, got)
		}
	}
}

// The key is filled even for a name the token comparison cannot identify — that
// is the whole point: those names are exactly the ones ZarFilm serves.
func TestReleaseOfCarriesAKeyEvenWhenUnidentifiable(t *testing.T) {
	r, ok := ReleaseOf("Coyote.vs.Acme.2026.1080p.WEBRip.Dubbed.ZarFilm.mkv")
	if ok {
		t.Fatal("this name names no group, so the token path must still refuse it")
	}
	if r.Key == "" {
		t.Fatal("but it must still carry a key")
	}
}

// Spec 0010: a library renamed for a media server keeps no release information,
// so the version has to come from the name recorded when the download was made.
// Those names put the SITE's brand last and the encoder immediately before it,
// which is not the shape ReleaseOf expects.
func TestRecordedReleaseTakesTheEncoderBeforeTheSiteTag(t *testing.T) {
	for _, tc := range []struct{ name, res, group string }{
		{"Show.S01E01.1080p.WEB-DL.x265.Silence-30nama.mkv", "1080p", "silence"},
		{"Show.S01E02.720p.BluRay.x264.PSA-30nama.mkv", "720p", "psa"},
		{"Movie.2024.2160p.WEB-DL.YIFY-30nama.mkv", "2160p", "yify"},
		{"Show S02E03 1080p BluRay Flux 30nama.mkv", "1080p", "flux"},
	} {
		got, ok := RecordedRelease(tc.name)
		if !ok {
			t.Fatalf("%s: not recovered", tc.name)
		}
		if got.Resolution != tc.res || got.Group != tc.group {
			t.Fatalf("%s: got %+v, want %s/%s", tc.name, got, tc.res, tc.group)
		}
	}
}

// Both halves are required, so a token picked out of the wrong place simply
// matches no option rather than marking a version the user does not have. That
// is what makes taking the encoder by position safe.
func TestRecordedReleaseNeedsBothHalves(t *testing.T) {
	for _, name := range []string{
		"Show.S01E01.mkv",         // no resolution
		"1080p.mkv",               // no token before the last
		"",                        //
		"Show - S01E01 - Uno.mkv", // renamed for a media server: nothing to find
	} {
		if got, ok := RecordedRelease(name); ok {
			t.Fatalf("%q should not be recovered, got %+v", name, got)
		}
	}
}

// A recovered release only marks an option when BOTH halves agree.
func TestRecoveredReleaseOnlyMatchesOnBothHalves(t *testing.T) {
	rel, ok := RecordedRelease("Show.S01E01.1080p.WEB-DL.x265.Silence-30nama.mkv")
	if !ok {
		t.Fatal("not recovered")
	}
	if !rel.Matches("1080p", "Silence") {
		t.Error("the option it came from must match")
	}
	if rel.Matches("1080p", "TENEIGHTY") {
		t.Error("same resolution, different encoder must not match")
	}
	if rel.Matches("720p", "Silence") {
		t.Error("same encoder, different resolution must not match")
	}
}

// Spec 2015: the encoder used to be taken by POSITION — "the token before the
// last" — which is a subtitle or dubbing marker on the names these sites
// actually publish. Every recorded download on the reporting instance recovered
// the wrong token, so the version match failed on exactly the downloads SynoDL
// had sent itself.
func TestRecordedReleaseSkipsSubtitleAndDubbingMarkers(t *testing.T) {
	for _, tc := range []struct{ name, res, group string }{
		// The token before the brand is the subtitle marker; the encoder is before it.
		{"The.Sheep.Detectives.2026.1080p.BluRay.x264.DD5.1.Pahe.SoftSub.ZarFilm.mkv", "1080p", "pahe"},
		{"Movie.2024.1080p.WEB-DL.x265.Joy.HardSub.ZarFilm.mkv", "1080p", "joy"},
		{"Show.S01E01.720p.WEB-DL.Silence.Dubbed.30nama.mkv", "720p", "silence"},
	} {
		got, ok := RecordedRelease(tc.name)
		if !ok {
			t.Errorf("%s: not recovered", tc.name)
			continue
		}
		if got.Resolution != tc.res || got.Group != tc.group {
			t.Errorf("%s: got %s/%s, want %s/%s", tc.name, got.Resolution, got.Group, tc.res, tc.group)
		}
	}
}

// A name that carries no encoder at all must recover none. Inventing one from a
// dubbing marker is how "Mutiny" reported an encoder its file never had.
func TestRecordedReleaseRecoversNothingWhenTheNameNamesNoEncoder(t *testing.T) {
	for _, name := range []string{
		"Mutiny.2026.1080p.FHD.WEB-DL.Dubbed.ZarFilm.mkv",
		"Batman.Knightfall.Part.1.Knightfall.2026.1080p.WEB-DL.Dubbed.mkv",
		"Movie.2024.1080p.BluRay.x264.DD5.1.mkv",
	} {
		if got, ok := RecordedRelease(name); ok {
			t.Errorf("%q recovered %s/%s; it names no encoder", name, got.Resolution, got.Group)
		}
	}
}

// One candidate after the resolution is ambiguous: on these sites it is the
// site's own brand. A hyphen is what distinguishes a scene group from a brand.
func TestRecordedReleaseTreatsALoneDottedTokenAsTheSiteBrand(t *testing.T) {
	if got, ok := RecordedRelease("Movie.2024.1080p.BluRay.x264.ZarFilm.mkv"); ok {
		t.Errorf("a lone dotted token is the site brand, not an encoder; recovered %s", got.Group)
	}
	got, ok := RecordedRelease("Movie.2024.1080p.BluRay.x264-RARBG.mkv")
	if !ok {
		t.Fatal("a scene-style -GROUP is a group, and must be recovered")
	}
	if got.Group != "rarbg" {
		t.Errorf("group = %q, want rarbg", got.Group)
	}
}

// The vocabulary deny-list decides what can be an encoder at all, so its edges
// are worth pinning: punctuation-only tokens and bare numbers are never groups,
// and an unknown word is (that is the point of a deny-list — group names are
// open-ended).
func TestReleaseVocabularyEdges(t *testing.T) {
	for _, tc := range []struct {
		tok  string
		want bool
	}{
		{"", true},        // nothing at all
		{"...", true},     // folds away to nothing
		{"5", true},       // a channel count, from DD5.1
		{"2024", true},    // a year
		{"1080p", true},   // the resolution itself
		{"WEB", true},     // known source tag
		{"SoftSub", true}, // known subtitle marker
		{"x265", true},    // known codec
		{"Pahe", false},   // a group
		{"TENEIGHTY", false},
		{"30nama", false}, // a brand, but indistinguishable from a group here
	} {
		if got := isReleaseVocabulary(tc.tok); got != tc.want {
			t.Errorf("isReleaseVocabulary(%q) = %v, want %v", tc.tok, got, tc.want)
		}
	}
}

// A resolution written as 4K/UHD is recognised for the RESOLUTION but does not
// anchor the token walk, so nothing is recovered rather than a token picked from
// the wrong place.
func TestRecordedReleaseWithoutAResolutionTokenRecoversNothing(t *testing.T) {
	for _, name := range []string{
		"Movie.2024.4K.WEB-DL.x265.Joy.ZarFilm.mkv",
		"Movie.2024.UHD.BluRay.Silence.30nama.mkv",
	} {
		if got, ok := RecordedRelease(name); ok {
			t.Errorf("%q recovered %s/%s; no resolution token anchors the walk", name, got.Resolution, got.Group)
		}
	}
}

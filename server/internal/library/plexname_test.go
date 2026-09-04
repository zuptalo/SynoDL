package library

import "testing"

// The Go twin of what scripts/library_tidy.py produces, so a freshly sent
// download lands with the same name the tidy script would have given it.
func TestPlexName(t *testing.T) {
	cases := []struct{ raw, want string }{
		// The everyday case: the sources put the year at the end, bare.
		{"Despicable Me 4 2024", "Despicable Me 4 (2024)"},
		{"Dune 2021", "Dune (2021)"},
		// A year range is a series' whole run; a scraper keys on the FIRST year.
		{"Friends 1994 - 2004", "Friends (1994)"},
		{"Breaking Bad 2008–2013", "Breaking Bad (2008)"},
		{"Severance 2022 -", "Severance (2022)"},
		// Already correct, and must not gain a second set of parentheses.
		{"Arrival (2016)", "Arrival (2016)"},
		// No year to move.
		{"Apocalipsis Z", "Apocalipsis Z"},
		// A title that IS a year must not be emptied into "()".
		{"1917", "1917"},
		{"2012", "2012"},
		// A title containing a year, plus a release year.
		{"Blade Runner 2049 2017", "Blade Runner 2049 (2017)"},
		{"1992 2024", "1992 (2024)"},
		// Punctuation and non-Latin scripts survive untouched.
		{"WALL-E 2008", "WALL-E (2008)"},
		{"Léon The Professional 1994", "Léon The Professional (1994)"},
		{"جدایی نادر از سیمین 2011", "جدایی نادر از سیمین (2011)"},
		{"", ""},
	}
	for _, c := range cases {
		if got := PlexName(c.raw); got != c.want {
			t.Errorf("PlexName(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// Running it over its own output must change nothing — SynoDL re-derives the
// name on every send, so a name that shifted each time would spawn a new folder.
func TestPlexNameIsAFixedPoint(t *testing.T) {
	for _, raw := range []string{
		"Despicable Me 4 2024", "Friends 1994 - 2004", "Arrival (2016)", "1917",
		"Blade Runner 2049 2017", "Apocalipsis Z", "Severance 2022 -",
	} {
		once := PlexName(raw)
		if twice := PlexName(once); twice != once {
			t.Errorf("PlexName(%q) = %q, but re-applying gives %q", raw, once, twice)
		}
	}
}

func TestSeasonFolder(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{1, "Season 01"}, {9, "Season 09"}, {10, "Season 10"}, {2024, "Season 2024"}} {
		if got := SeasonFolder(c.n); got != c.want {
			t.Errorf("SeasonFolder(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// The season comes from the files being downloaded, never from the client, so
// it cannot disagree with what actually lands on disk (FR-007).
func TestSeasonOfFiles(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  int
		ok    bool
	}{
		{"plain", []string{"Friends.S01E01.1080p.mkv", "Friends.S01E02.1080p.mkv"}, 1, true},
		{"underscores", []string{"X_Men_97_S02E03_1080p_WEB-DL.mkv"}, 2, true},
		{"alternate form", []string{"Show.1x05.HDTV.mkv"}, 1, true},
		{"double digit", []string{"Show.S10E01.mkv"}, 10, true},
		{"full urls", []string{"https://cdn.example/dl/Friends.S03E04.mkv?token=abc"}, 3, true},
		{"a movie has none", []string{"Dune.2021.2160p.WEB-DL.mkv"}, 0, false},
		{"nothing parseable", []string{"episode-one.mkv"}, 0, false},
		{"none at all", nil, 0, false},
		// A pack spanning seasons is ambiguous: refuse rather than pick one and
		// file half the files under the wrong season (FR-006).
		{"mixed seasons", []string{"Show.S01E01.mkv", "Show.S02E01.mkv"}, 0, false},
		// A year in the name must not be mistaken for a season.
		{"year not a season", []string{"Show.2019.1080p.mkv"}, 0, false},
	}
	for _, c := range cases {
		got, ok := SeasonOfFiles(c.files)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: SeasonOfFiles(%v) = (%d, %v), want (%d, %v)", c.name, c.files, got, ok, c.want, c.ok)
		}
	}
}

// Episode numbers come from the FILES (FR-016), so this is what makes "which
// episodes do I have" answerable at all. The regex already found the season and
// threw the episode away.
func TestEpisodeOf(t *testing.T) {
	cases := []struct {
		name    string
		season  int
		episode int
		ok      bool
	}{
		{"Show.S01E02.1080p.mkv", 1, 2, true},
		{"Show.s1.e5.mkv", 1, 5, true},
		{"Show 1x05 720p.mkv", 1, 5, true},
		// Underscores are why the regex bounds on non-alphanumerics rather than \b.
		{"X_Men_97_S02E03_1080p_30NAMA.mkv", 2, 3, true},
		{"Attack_on_Titan_S00E01_BluRay.mkv", 0, 1, true},
		{"Show.S01E123.mkv", 1, 123, true},
		// A signed link's query is full of digits and must not be mined for one.
		{"Show.S01E02.mkv?md5=7Iayb7PJyj4&expires=1788", 1, 2, true},
		{"Season 01", 0, 0, false},
		{"just-a-movie-2021.mkv", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		s, e, ok := EpisodeOf(c.name)
		if ok != c.ok || (ok && (s != c.season || e != c.episode)) {
			t.Errorf("EpisodeOf(%q) = (%d,%d,%v), want (%d,%d,%v)",
				c.name, s, e, ok, c.season, c.episode, c.ok)
		}
	}
}

// SeasonOfFiles reads the same regex; adding episode capture must not shift the
// season it reports.
func TestSeasonOfFilesStillAgreesAfterEpisodeCapture(t *testing.T) {
	if n, ok := SeasonOfFiles([]string{"Show.S03E01.mkv", "Show.S03E02.mkv"}); !ok || n != 3 {
		t.Errorf("SeasonOfFiles = (%d,%v), want (3,true)", n, ok)
	}
	if n, ok := SeasonOfFiles([]string{"Show 2x01.mkv"}); !ok || n != 2 {
		t.Errorf("SeasonOfFiles x-form = (%d,%v), want (2,true)", n, ok)
	}
	if _, ok := SeasonOfFiles([]string{"Show.S01E01.mkv", "Show.S02E01.mkv"}); ok {
		t.Error("a set spanning seasons must report unknown")
	}
}

func TestSeasonOfFolder(t *testing.T) {
	for _, c := range []struct {
		name   string
		season int
		ok     bool
	}{
		{"Season 01", 1, true},
		{"Season 1", 1, true},
		{"season 12", 12, true},
		{"S01", 1, true},
		{"Series 2", 2, true},
		{"Season 00", 0, true},
		{"Specials", 0, true},
		{"specials", 0, true},
		// A real title starting with the word must not be read as a season.
		{"Seasons of Love", 0, false},
		{"Season", 0, false},
		{"Friends (1994)", 0, false},
		{"", 0, false},
	} {
		n, ok := SeasonOfFolder(c.name)
		if ok != c.ok || (ok && n != c.season) {
			t.Errorf("SeasonOfFolder(%q) = (%d,%v), want (%d,%v)", c.name, n, ok, c.season, c.ok)
		}
	}
}

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

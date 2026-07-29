package tasktitle

import "testing"

// These cases mirror src/services/task-title.test.ts so the Go port and the TS
// original stay behaviourally identical.
func TestTitle(t *testing.T) {
	cases := []struct {
		name, dest, uri string
		wantTitle       string
		wantEpisode     string
	}{
		{
			name: "file.mkv", dest: "movies/Despicable Me 4 2024",
			wantTitle: "Despicable Me 4 2024", wantEpisode: "",
		},
		{
			name: "Rick.and.Morty.S01E05.1080p.WEB-DL.mkv", dest: "tv-show/Rick and Morty",
			wantTitle: "Rick and Morty", wantEpisode: "S01E05",
		},
		{
			name: "X_Men_97_S01E01_1080p_WEB-DL_TheCuteness_30NAMA.mkv", dest: "series/x_men_97",
			wantTitle: "x_men_97", wantEpisode: "S01E01",
		},
		{
			name: "The_Big_Bang_Theory_S02E01_10bit_x265_1080p_BluRay_RCVR_30NAMA.mkv", dest: "series/the_big_bang_theory",
			wantTitle: "the_big_bang_theory", wantEpisode: "S02E01",
		},
		{
			name: "opaque", dest: "series/x_men_97",
			uri:       "https://host/download/.../series/x_men_97/X_Men_97_S01E10_1080p_WEB-DL_30NAMA.mkv",
			wantTitle: "x_men_97", wantEpisode: "S01E10",
		},
		{
			name: "ep.mkv", dest: "tv/Show", uri: "https://cdn/x/s2e7/file.mkv",
			wantTitle: "Show", wantEpisode: "S02E07",
		},
		{
			name: "linux.iso", dest: "",
			wantTitle: "linux.iso", wantEpisode: "",
		},
		{
			name: "e2e-fixture.iso", dest: "home/Downloads",
			wantTitle: "e2e-fixture.iso", wantEpisode: "",
		},
	}
	for _, c := range cases {
		title, episode := Title(c.name, c.dest, c.uri)
		if title != c.wantTitle || episode != c.wantEpisode {
			t.Errorf("Title(%q,%q,%q) = (%q,%q), want (%q,%q)",
				c.name, c.dest, c.uri, title, episode, c.wantTitle, c.wantEpisode)
		}
	}
}

func TestDisplay(t *testing.T) {
	if got := Display("Rick.and.Morty.S01E05.mkv", "tv/Rick and Morty", ""); got != "Rick and Morty · S01E05" {
		t.Errorf("Display series = %q", got)
	}
	if got := Display("file.mkv", "movies/Dune 2024", ""); got != "Dune 2024" {
		t.Errorf("Display movie = %q", got)
	}
	if got := Display("linux.iso", "", ""); got != "linux.iso" {
		t.Errorf("Display fallback = %q", got)
	}
}

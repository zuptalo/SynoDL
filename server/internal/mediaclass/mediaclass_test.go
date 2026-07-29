package mediaclass

import (
	"testing"

	"synodl/server/internal/store"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		dest, file string
		want       string
	}{
		{"movies/Dune 2024", "Dune.2024.mkv", store.CategoryMovie},
		{"tv/The Bear", "The.Bear.S01E01.mkv", store.CategorySeries},
		{"tv-shows/Show", "ep.mkv", store.CategorySeries},
		{"anime/X Men 97", "X_Men_97_S01E01.mkv", store.CategoryAnime},
		{"music-videos/Artist", "clip.mp4", store.CategoryMusicVideo},
		{"music/Artist/Album", "cover.jpg", store.CategoryMusic}, // music folder, non-audio → trust folder
		// Audio extension always wins, wherever it lands.
		{"Downloads", "track.mp3", store.CategoryMusic},
		{"movies/x", "bonus.flac", store.CategoryMusic},
		// No folder signal, non-audio → other.
		{"Downloads", "ubuntu.iso", store.CategoryOther},
		{"", "something", store.CategoryOther},
	}
	for _, c := range cases {
		if got := Classify(c.dest, c.file); got != c.want {
			t.Errorf("Classify(%q, %q) = %q, want %q", c.dest, c.file, got, c.want)
		}
	}
}

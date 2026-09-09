package library

import "testing"

func TestMusicSegment_RefusesWhatCannotBeMadeSafe(t *testing.T) {
	// The one that matters. A bare ".." holds no separator to replace, so most
	// cleaning leaves it exactly as it was — and rendered into a path it climbs
	// out of the library. Spec 0013 found this in the download recipe; the same
	// value can now be TYPED, which is a shorter route to it.
	for _, in := range []string{"..", ".", "...", "   ", "", ". . .", "\x00", "/"} {
		if got := MusicSegment(in); got != "" {
			t.Errorf("MusicSegment(%q) = %q, want a refusal", in, got)
		}
	}
}

func TestMusicSegment_KeepsAUsableName(t *testing.T) {
	cases := map[string]string{
		"Anyma":                "Anyma",
		"  Anyma  ":            "Anyma",
		"AC/DC":                "AC DC",
		`Anyma: "Genesys"`:     "Anyma Genesys",
		"Sigur Rós":            "Sigur Rós",
		"Anyma\\Chris":         "Anyma Chris",
		"Track  with   spaces": "Track with spaces",
		"Trailing dots...":     "Trailing dots",
		"...Leading dots":      "Leading dots",
		"日本語のタイトル":             "日本語のタイトル",
	}
	for in, want := range cases {
		if got := MusicSegment(in); got != want {
			t.Errorf("MusicSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMusicSegment_Truncates(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "a"
	}
	if got := MusicSegment(long); len(got) != 120 {
		t.Errorf("length = %d, want it capped at 120", len(got))
	}
}

func TestMusicFolders(t *testing.T) {
	artist, album, ok := MusicFolders("Anyma", "The End Of Genesys")
	if !ok || artist != "Anyma" || album != "The End Of Genesys" {
		t.Fatalf("got %q/%q ok=%v", artist, album, ok)
	}

	// No album is not an error: it is the Singles case, and it must use the same
	// word the download recipe falls back to or uploaded and downloaded singles
	// end up in two different folders.
	_, album, ok = MusicFolders("Anyma", "")
	if !ok || album != "Singles" {
		t.Fatalf("no album gave %q ok=%v, want Singles", album, ok)
	}
	// An album that sanitises to nothing is the same answer as no album.
	if _, album, _ = MusicFolders("Anyma", ".."); album != "Singles" {
		t.Errorf("unusable album gave %q, want Singles", album)
	}

	// An artist has no fallback: a track has to belong to somebody.
	if _, _, ok = MusicFolders("..", "Album"); ok {
		t.Error("an unusable artist was accepted; it would have escaped the library")
	}
	if _, _, ok = MusicFolders("", "Album"); ok {
		t.Error("an empty artist was accepted")
	}
}

func TestMusicFileName_MatchesLyricsToTheirAudio(t *testing.T) {
	// A media server pairs a .lrc to its audio by IDENTICAL base name. Uploading
	// them under two different names is the same as not uploading the lyrics.
	audio, ok := MusicFileName("Lucente", "whatever-the-phone-called-it.mp3")
	if !ok || audio != "Lucente.mp3" {
		t.Fatalf("audio = %q ok=%v, want Lucente.mp3", audio, ok)
	}
	lyrics, ok := MusicFileName("Lucente", "some-download.LRC")
	if !ok || lyrics != "Lucente.lrc" {
		t.Fatalf("lyrics = %q ok=%v, want Lucente.lrc", lyrics, ok)
	}
}

func TestMusicFileName_ArtworkBecomesTheAlbumCover(t *testing.T) {
	// Artwork belongs to the ALBUM, not the track, so it is the one file that is
	// not named after the track.
	for _, in := range []string{"anything.jpg", "IMG_1234.PNG", "art.webp"} {
		got, ok := MusicFileName("Lucente", in)
		if !ok || !hasPrefix(got, "cover.") {
			t.Errorf("MusicFileName(%q) = %q ok=%v, want a cover.*", in, got, ok)
		}
	}
}

func TestMusicFileName_TakesItsTypeFromTheFileNotTheField(t *testing.T) {
	// The track name supplies a BASE, never a suffix. Otherwise typing
	// "song.mp3" into the field would be a way to end up with a different
	// extension than the one that was type-checked.
	got, ok := MusicFileName("Lucente.mp3", "upload.flac")
	if !ok || got != "Lucente.mp3.flac" {
		t.Fatalf("got %q ok=%v, want the field used as a base only", got, ok)
	}
}

func TestMusicFileName_Refuses(t *testing.T) {
	if _, ok := MusicFileName("Lucente", "no-extension"); ok {
		t.Error("a file with no extension was accepted")
	}
	if _, ok := MusicFileName("..", "track.mp3"); ok {
		t.Error("an unusable track name was accepted")
	}
	if _, ok := MusicFileName("", "track.mp3"); ok {
		t.Error("an empty track name was accepted")
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

// Each library holds what it is for. Narrowness is the whole point of an
// allowlist, so a film upload must not accept an .mp3 and a music upload must
// not accept an .mkv.
func TestAllowedUploadTypeFor(t *testing.T) {
	cases := []struct {
		kind UploadKind
		name string
		want bool
	}{
		{KindMovie, "film.mkv", true},
		{KindMovie, "film.mp3", false},
		{KindMovie, "film.srt", true},
		{KindTV, "ep.mp4", true},
		{KindTV, "ep.flac", false},

		{KindMusic, "track.mp3", true},
		{KindMusic, "track.flac", true},
		{KindMusic, "track.m4a", true},
		{KindMusic, "track.lrc", true},
		{KindMusic, "cover.jpg", true},
		{KindMusic, "track.mkv", false},

		{KindMusicVideo, "clip.mp4", true},
		{KindMusicVideo, "clip.mkv", true},
		{KindMusicVideo, "clip.lrc", true},
		{KindMusicVideo, "clip.mp3", false},

		{KindMusic, "noextension", false},
		{KindMusic, "script.sh", false},
		{KindMovie, "script.sh", false},
	}
	for _, c := range cases {
		if got := AllowedUploadTypeFor(c.kind, c.name); got != c.want {
			t.Errorf("AllowedUploadTypeFor(%q, %q) = %v, want %v", c.kind, c.name, got, c.want)
		}
	}
}

func TestIsMusicKind(t *testing.T) {
	for k, want := range map[UploadKind]bool{
		KindMusic: true, KindMusicVideo: true, KindMovie: false, KindTV: false, "": false,
	} {
		if got := IsMusicKind(k); got != want {
			t.Errorf("IsMusicKind(%q) = %v, want %v", k, got, want)
		}
	}
}

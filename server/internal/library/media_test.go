package library

import "testing"

// Ownership turns on this and nothing else (FR-001a): a folder full of artwork,
// subtitles and .nfo holds no content, and 0.3.0 marked exactly that as owned.
func TestIsVideo(t *testing.T) {
	video := []string{
		"Dune (2021).mkv", "film.mp4", "a.avi", "b.m4v", "c.mov", "d.wmv",
		"e.mpg", "f.mpeg", "g.m2ts", "h.ts", "i.webm", "j.vob",
		"SHOUTING.MKV", "Mixed.Case.Mp4",
	}
	for _, name := range video {
		if !IsVideo(name) {
			t.Errorf("IsVideo(%q) = false, want true", name)
		}
	}

	// Sidecars are the whole point of the rule: they are legitimately present
	// beside content and are never evidence of it.
	notVideo := []string{
		"season.nfo", "movie.nfo", "poster.jpg", "fanart.png", "art.webp",
		"subs.srt", "subs.ass", "subs.vtt", "idx.idx", "meta.xml",
		"README", "", ".mkv.txt", "trailer.mkv.part",
	}
	for _, name := range notVideo {
		if IsVideo(name) {
			t.Errorf("IsVideo(%q) = true, want false", name)
		}
	}
}

// A name with no extension at all must not slip through on a prefix match.
func TestIsVideoNeedsARealExtension(t *testing.T) {
	for _, name := range []string{"mkv", ".mkv", "Season 01", "mp4."} {
		if IsVideo(name) {
			t.Errorf("IsVideo(%q) = true, want false", name)
		}
	}
}

// Uploads must keep accepting sidecars; splitting video out of the table must not
// narrow what may be uploaded.
func TestSplittingVideoOutKeepsUploadsUnchanged(t *testing.T) {
	for _, name := range []string{"a.mkv", "a.srt", "a.jpg", "a.nfo", "a.xml"} {
		if !AllowedUploadType(name) {
			t.Errorf("AllowedUploadType(%q) = false; the upload rule changed", name)
		}
	}
	if AllowedUploadType("a.exe") {
		t.Error("AllowedUploadType(a.exe) = true")
	}
}

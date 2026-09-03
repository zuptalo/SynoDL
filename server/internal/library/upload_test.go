package library

import "testing"

// The file name is client-supplied text that becomes part of a path on the NAS.
// The title is sanitised; this one is deliberately preserved, which makes it the
// easier of the two to forget.
func TestValidUploadName(t *testing.T) {
	for _, ok := range []string{
		"Friends.S01E01.1080p.mkv", "Dune (2021).mkv", "poster.jpg",
		"فیلم.mkv", "a b  c.srt", "x.mkv",
	} {
		if !ValidUploadName(ok) {
			t.Errorf("ValidUploadName(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{
		"", "   ", ".", "..",
		"../../etc/passwd", "a/b.mkv", `a\b.mkv`, "/abs.mkv",
		"tab\there.mkv", "null\x00.mkv", "nl\n.mkv",
	} {
		if ValidUploadName(bad) {
			t.Errorf("ValidUploadName(%q) = true, want false", bad)
		}
	}
}

func TestValidUploadNameRejectsOverlongNames(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "a"
	}
	if ValidUploadName(long + ".mkv") {
		t.Error("a 300-character name should be refused")
	}
}

// Restricting the types is what keeps this a media-upload feature rather than a
// general write-anything-to-the-NAS tool.
func TestAllowedUploadType(t *testing.T) {
	for _, ok := range []string{
		"ep.mkv", "ep.MP4", "movie.avi", "subs.srt", "subs.ASS",
		"poster.jpg", "fanart.png", "info.nfo", "meta.xml",
	} {
		if !AllowedUploadType(ok) {
			t.Errorf("AllowedUploadType(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{
		"script.sh", "payload.exe", "archive.zip", "page.html", "lib.so",
		"noextension", "", "trailingdot.",
	} {
		if AllowedUploadType(bad) {
			t.Errorf("AllowedUploadType(%q) = true, want false", bad)
		}
	}
}

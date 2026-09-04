package library

import (
	"path"
	"strings"
)

// videoExt is what counts as CONTENT. Everything else a library folder legitimately
// contains — subtitles, artwork, .nfo — sits beside content without being it, which
// is the distinction FR-001a turns on: a folder holding only sidecars holds nothing
// watchable, and reporting it as owned is what this rule exists to prevent.
//
// The same table governs uploads (see uploadExt), so a file recognised as video on
// the way in is recognised as video on the way out.
var videoExt = map[string]bool{
	"mkv": true, "mp4": true, "avi": true, "m4v": true, "mov": true, "wmv": true,
	"mpg": true, "mpeg": true, "m2ts": true, "ts": true, "webm": true, "vob": true,
}

// ext returns a file name's lower-cased extension, or "" when it has no real one.
//
// A name that is ALL extension (".mkv") is a dotfile with no stem, not a video, and
// a trailing dot ("mp4.") is not an extension either. Both would otherwise be read
// as content on a prefix match.
func ext(name string) string {
	base := path.Base(strings.TrimSpace(name))
	e := path.Ext(base)
	if e == "" || e == base {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(e, "."))
}

// IsVideo reports whether a file name is the kind of file a library is made of.
//
// This is the whole of the ownership test: a title folder is present when it holds
// at least one of these, and is NOT present when it holds only sidecars, however
// many (FR-001, FR-001a).
func IsVideo(name string) bool {
	e := ext(name)
	return e != "" && videoExt[e]
}

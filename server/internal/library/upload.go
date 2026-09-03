package library

import (
	"path"
	"strings"
)

// Validating what may be uploaded, and under what name (spec 1022).
//
// Two client-supplied strings reach a path on the NAS: the title, which is
// sanitised into a single folder segment by the caller, and the FILE NAME, which
// is deliberately preserved so the file keeps the name its owner gave it. The
// second is the easy one to overlook — "preserve the original name" reads as a
// courtesy rather than as accepting untrusted input into a path — so it is
// validated here and REFUSED when it is not a single plain segment, never
// repaired into something else.

// uploadExt is what an upload may carry: the media a library is made of, and the
// sidecars that belong beside it. The restriction is the point. Without it the
// endpoint is a general "write any file to the NAS" tool, which is a far larger
// capability than the feature asks for and one an operator has not knowingly
// granted.
var uploadExt = map[string]bool{
	// Video.
	"mkv": true, "mp4": true, "avi": true, "m4v": true, "mov": true, "wmv": true,
	"mpg": true, "mpeg": true, "m2ts": true, "ts": true, "webm": true, "vob": true,
	// Subtitles.
	"srt": true, "sub": true, "idx": true, "ass": true, "ssa": true, "vtt": true,
	"smi": true, "sup": true,
	// Artwork.
	"jpg": true, "jpeg": true, "png": true, "webp": true, "tbn": true, "bmp": true,
	// Metadata.
	"nfo": true, "xml": true,
}

// ValidUploadName reports whether a client-supplied file name may be used as-is.
//
// It must be a single plain segment: no path separator, no parent-directory
// reference, no control character, and not one of the dot entries. A name that
// fails is rejected rather than cleaned up, because silently rewriting a hostile
// name and then writing to wherever the rewrite landed is the failure this
// guards against.
func ValidUploadName(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || n == "." || n == ".." || len(n) > 255 {
		return false
	}
	if strings.ContainsAny(n, `/\`) {
		return false
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	// Belt and braces: whatever the string looked like, its own base must be
	// itself, or it was describing a path rather than a name.
	return path.Base(n) == n
}

// AllowedUploadType reports whether a file's extension is one a media library
// is made of. Compared lower-case, and an extension-less name is refused —
// there is nothing to check it against.
func AllowedUploadType(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(name)), "."))
	return ext != "" && uploadExt[ext]
}

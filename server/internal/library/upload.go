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
// Composed so VIDEO has exactly one definition (videoExt in media.go, which
// ownership also reads) and the sidecars are listed once here. Two hand-kept lists
// would drift, and a file could then be uploadable but not recognised as content.
var uploadExt = func() map[string]bool {
	m := map[string]bool{}
	for e := range videoExt {
		m[e] = true
	}
	for _, e := range sidecarExt {
		m[e] = true
	}
	return m
}()

// audioExt is what a MUSIC library is made of (spec 1040).
//
// Its own set rather than more entries in uploadExt, because the check is
// per-kind: a film upload has no business accepting an .mp3, and a music upload
// none accepting an .mkv. Narrow is the whole point of having an allowlist.
var audioExt = map[string]bool{
	"mp3": true, "m4a": true, "flac": true, "opus": true, "ogg": true,
	"oga": true, "wav": true, "aac": true, "alac": true, "wma": true, "aiff": true,
}

// lyricsExt is the sidecar a track has and a film does not. It is matched to its
// audio by identical base name, which is why MusicFileName renames it.
var lyricsExt = map[string]bool{"lrc": true, "txt": true}

// sidecarExt belongs beside content without being content.
var sidecarExt = []string{
	// Subtitles.
	"srt", "sub", "idx", "ass", "ssa", "vtt", "smi", "sup",
	// Artwork.
	"jpg", "jpeg", "png", "webp", "tbn", "bmp",
	// Metadata.
	"nfo", "xml",
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

// UploadKind is what an upload says it is. The kind decides which file types
// are acceptable, which is what keeps each library holding what it is for.
type UploadKind string

const (
	KindMovie      UploadKind = "movie"
	KindTV         UploadKind = "tv"
	KindMusic      UploadKind = "music"
	KindMusicVideo UploadKind = "music-video"
)

// IsMusicKind reports whether an upload is going to a music library.
func IsMusicKind(k UploadKind) bool { return k == KindMusic || k == KindMusicVideo }

// AllowedUploadTypeFor reports whether a file belongs in this KIND of upload
// (spec 1040).
//
// Every kind takes the sidecars — subtitles, artwork, metadata belong beside any
// content — and then exactly one kind of content: audio for music, video for
// everything else. A music upload additionally takes lyrics, which a film has no
// use for.
func AllowedUploadTypeFor(kind UploadKind, name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(name)), "."))
	if ext == "" {
		return false
	}
	if sidecarSet[ext] {
		return true
	}
	if kind == KindMusic {
		return audioExt[ext] || lyricsExt[ext]
	}
	if kind == KindMusicVideo {
		return videoExt[ext] || lyricsExt[ext]
	}
	return videoExt[ext]
}

// sidecarSet is sidecarExt as a lookup.
var sidecarSet = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range sidecarExt {
		m[e] = true
	}
	return m
}()

// AllowedUploadType reports whether a file's extension is one a media library
// is made of. Compared lower-case, and an extension-less name is refused —
// there is nothing to check it against.
func AllowedUploadType(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(name)), "."))
	return ext != "" && uploadExt[ext]
}

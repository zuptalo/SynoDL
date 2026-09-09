package library

import (
	"path"
	"strings"
)

// Where an uploaded track goes, and what it is called (spec 1040).
//
// The layout is NOT a new decision. Spec 0012's shipped yt-dlp recipe files a
// downloaded track as artist / album / title, falling back to "Singles" when the
// source publishes no album, and this composes exactly the same thing — so an
// uploaded track and a downloaded one sit together rather than in two parallel
// shapes, which is the rule spec 1022 already holds for films.
//
// Everything here is pure and refuses rather than repairs. Three client-supplied
// strings become path segments, and a segment that cannot be made safe is an
// error the user can act on, not something quietly rewritten and then written to
// wherever the rewrite landed.

// NoAlbum is where a track with no album given is filed.
//
// The same literal the download recipe falls back to (`%(album|Singles)s`). It
// matters that it is the same word: two spellings would put uploaded and
// downloaded singles in two different folders.
const NoAlbum = "Singles"

// MusicSegment sanitises one client-supplied value into a single path segment.
//
// Returns "" when nothing usable is left, which the caller must treat as a
// refusal. That is the case worth being careful about: a value of ".." or "..."
// survives most cleaning — it holds no separator to replace — and rendered into
// a path it climbs out of the library. Spec 0013 found exactly that in the
// download recipe and had to guard it there too.
func MusicSegment(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return ' '
		case r < 0x20 || r == 0x7f:
			return ' '
		default:
			return r
		}
	}, s)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	// Trailing dots and spaces go for Windows/SMB's sake, and that is also what
	// reduces ".." to nothing. Leading ones go for tidiness rather than safety —
	// a name starting with dots is not a traversal once it holds no separator —
	// so do not read this line as the guard. The guard is the check below.
	cleaned = strings.Trim(cleaned, " .")
	if len(cleaned) > 120 {
		cleaned = strings.TrimSpace(cleaned[:120])
	}
	// Belt and braces: whatever survived, its own base must be itself.
	if cleaned == "" || cleaned == "." || cleaned == ".." || path.Base(cleaned) != cleaned {
		return ""
	}
	return cleaned
}

// MusicFolders is the artist and album folders a track belongs in.
//
// `ok` is false when the artist cannot be made into a safe segment, because an
// artist is the one part with no fallback: a track has to belong to somebody. An
// unusable album falls back to Singles rather than failing, which is the same
// answer as no album at all.
func MusicFolders(artist, album string) (artistFolder, albumFolder string, ok bool) {
	artistFolder = MusicSegment(artist)
	if artistFolder == "" {
		return "", "", false
	}
	albumFolder = MusicSegment(album)
	if albumFolder == "" {
		albumFolder = NoAlbum
	}
	return artistFolder, albumFolder, true
}

// coverName is what a media server looks for as an album's artwork. Both Plex
// and Jellyfin match it by this name in the album's own folder, which is why the
// thumbnail is not named after the track like everything else in the upload.
const coverName = "cover"

// MusicFileName is what one uploaded file is stored as.
//
// The track's name for the audio and anything that must be MATCHED to it — a
// media server pairs a .lrc to its audio by identical base name, so uploading
// them under two different names is the same as not uploading the lyrics at all.
//
// Artwork is the exception and takes the cover name instead, because it belongs
// to the album rather than to the track.
//
// The extension comes from the ORIGINAL file, lower-cased, and is never taken
// from anything the user typed: the track name supplies a base, never a suffix,
// so "Lucente.mp3" typed into the track field yields "Lucente.mp3.mp3" rather
// than a way to choose a different type than the one that was checked.
func MusicFileName(track, original string) (string, bool) {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(original)), "."))
	if ext == "" {
		return "", false
	}
	base := coverName
	if !IsArtwork(original) {
		base = MusicSegment(track)
		if base == "" {
			return "", false
		}
	}
	name := base + "." + ext
	if !ValidUploadName(name) {
		return "", false
	}
	return name, true
}

// IsArtwork reports whether a file is a picture rather than content or a
// sidecar that must match the track's name.
func IsArtwork(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(name)), "."))
	return artworkExt[ext]
}

var artworkExt = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "webp": true, "bmp": true, "tbn": true,
}

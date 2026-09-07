package ytdl

import (
	"fmt"
	"path"
)

// The output template and the album metadata template are THE SAME EXPRESSION.
// That identity is the whole reason a track cannot end up in a folder called
// one thing and tagged another — and it is why they are constants spliced into
// both places rather than two strings that happen to match today.
const (
	tmplArtist = "%(artist,uploader)s"
	tmplAlbum  = "%(album|Singles)s"
	tmplTitle  = "%(track,title)s"

	// OutputTemplate files everything as artist / album-or-Singles / title.
	// The title is used VERBATIM: the point of these downloads is that the item
	// can still be found back at its source later.
	OutputTemplate = tmplArtist + "/" + tmplAlbum + "/" + tmplTitle + ".%(ext)s"

	// VideoFormat prefers H.264 + AAC — not for quality, but because it direct-
	// plays on effectively every Plex client. The fallbacks keep an item that
	// offers neither from failing outright.
	//
	// This deliberately does NOT ask for a stream that already carries audio.
	// On YouTube only one such format exists (640x360), and it is served
	// intermittently, so a progressive-only selector fails at random. Combining
	// separate streams is a container operation, not a re-encode.
	VideoFormat = "bv*[vcodec^=avc1]+ba[acodec^=mp4a]/bv*+ba/b"

	// archiveName lives INSIDE the target library, so an item fetched as audio
	// does not suppress fetching it as video, or vice versa.
	archiveName = ".ytdlp-archive.txt"
)

// The two --exec snippets are the only shell in the entire command. They are
// CONSTANTS authored here, they contain no user input, and they receive the
// file path through the downloader's own quoted substitution (%(filepath)q).
// The submitted URL never appears in them. See TestArgs_ExecSnippetIsConstant…
//
// They exist because the downloader always names a companion file
// <base>.<lang>.<ext> and offers no way to omit the language infix — while Plex
// and Jellyfin match LYRICS by exact basename and video SUBTITLES by a 2-letter
// language suffix. Two different rules, one rename each.
//
// Both END IN `exit 0`, and that is load-bearing (spec 2020). An unmatched glob
// stays literal in a shell, so with no companion file the loop's last command
// is a false test — and a loop's exit status is its last command's. yt-dlp
// reads any non-zero exec status as post-processing failure and fails the
// download, which meant every track WITHOUT lyrics was reported as failed even
// though it had downloaded, tagged and saved perfectly. Lyrics are absent more
// often than present, so that was most of them.
//
// Tidying a sidecar is not worth failing a download over under any
// circumstance: the media file is the download.
const (
	execRenameLyrics = `after_move:p=%(filepath)q; b="${p%.mp3}"; for s in "$b".*.lrc; do [ -e "$s" ] && mv -f "$s" "$b.lrc"; done; exit 0`
	execRenameSubs   = `after_move:p=%(filepath)q; b="${p%.*}"; for s in "$b".*.srt; do n=$(echo "$s" | sed "s/-orig\.srt$/.srt/"); [ "$s" != "$n" ] && mv -f "$s" "$n"; done; exit 0`
)

// Options is everything needed to build a worker command.
type Options struct {
	Mode   Mode
	Target Target
	// OutDir is where the single mounted library appears inside the pod.
	OutDir string
	// MinDurationSeconds excludes clips from bulk runs. It is a LOWER bound
	// only: a long compilation is legitimate content.
	MinDurationSeconds int
}

// Args builds the worker's argv.
//
// The user-supplied URL is the final element, preceded by the end-of-options
// marker, and appears exactly once. Nothing in this function concatenates it
// into another string — that is a constitution v2.1.0 rule and there is a test
// that fails if it is ever broken.
func Args(o Options) []string {
	outDir := o.OutDir
	if outDir == "" {
		outDir = "/out"
	}
	minDur := o.MinDurationSeconds
	if minDur <= 0 {
		minDur = 90
	}

	args := []string{
		// Tags, not folders, are what a media server shelves by. Without these
		// two the album and album-artist tags come out EMPTY and every track
		// lands under "[Unknown Album]" despite a correct folder.
		"--parse-metadata", tmplAlbum + ":%(meta_album)s",
		"--parse-metadata", tmplArtist + ":%(meta_album_artist)s",
		"--embed-metadata",
		"--embed-thumbnail",
		"--convert-thumbnails", "jpg",
		// ".*-orig" is a regex, and it resolves the source's ORIGINAL-language
		// caption track without a metadata pre-pass.
		"--write-auto-subs",
		"--sub-langs", ".*-orig",
		"-P", outDir,
		"-o", OutputTemplate,
	}

	switch o.Mode {
	case ModeMusicVideo:
		args = append(args,
			"-f", VideoFormat,
			"--merge-output-format", "mp4",
			"--convert-subs", "srt",
			"--exec", execRenameSubs,
		)
	default: // ModeMusic
		args = append(args,
			"-x",
			"--audio-format", "mp3",
			"--audio-quality", "0",
			"--convert-subs", "lrc",
			"--exec", execRenameLyrics,
		)
	}

	switch o.Target.Scope {
	case ScopePlaylist:
		args = append(args,
			"--yes-playlist",
			"-i", // one private or deleted entry must not fail the whole run
			"--match-filter", fmt.Sprintf("duration >= %d", minDur),
			"--download-archive", path.Join(outDir, archiveName),
		)
	case ScopeChannel:
		args = append(args,
			"-i",
			"--match-filter", fmt.Sprintf("duration >= %d", minDur),
			"--download-archive", path.Join(outDir, archiveName),
			// A channel run makes many more requests than a single item; pacing
			// them keeps it from tripping the source's rate limiting part-way.
			"--sleep-requests", "1",
			"--sleep-interval", "3",
			"--max-sleep-interval", "8",
		)
	default: // ScopeSingle
		// An explicitly submitted link is an explicit choice: no archive and no
		// duration filter, so asking for a 40-second track still works.
		args = append(args, "--no-playlist")
	}

	// End-of-options, then the URL. The marker means a URL that somehow began
	// with "-" could never be parsed as a flag.
	return append(args, "--", o.Target.URL)
}

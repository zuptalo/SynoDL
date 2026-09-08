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
	// tmplAlbum is what the ITEM says its album is, or a generic.
	//
	// playlist_title is deliberately NOT in this chain, and spec 0012's research
	// is why: on a channel run it tags every track with the channel's TAB name
	// ("Lo-fi Beats - Videos"), which is worse than no album at all. There is a
	// test that fails if it ever reappears.
	//
	// Spec 0013's FR-036 — fall back to the playlist or channel name — is met a
	// different way: the SERVER knows that name, sanitises it, and passes it in
	// as a literal album (see Options.GroupName). That name is the real one
	// rather than a tab's, and it works for an expanded item, which has no
	// playlist context of its own at all.
	tmplAlbum = "%(album|Singles)s"
	tmplTitle = "%(track,title)s"

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

// directoryFields are the metadata fields that become DIRECTORY components via
// the output template.
//
// Each is set by the SOURCE, not by SynoDL — an artist name, an album name,
// read out of whatever the site published.
//
// `track` and `title` are deliberately NOT here. They become the FILENAME, which
// always has an extension appended, so a title of ".." yields "...mp3" — a file,
// not a traversal. Guarding them would also break 0012 FR-008, which requires
// the published title to survive verbatim so the item can be found back at its
// source later. There is a test that fails if that is ever rewritten.
var directoryFields = []string{"artist", "uploader", "album"}

// dotOnlyPattern matches a field made of nothing but dots and whitespace.
const dotOnlyPattern = `^[.\s]+$`

// dotOnlyGuardArgs neuters a source-derived field that is only dots, before it
// can become a path component (spec 0013, FR-038a).
//
// This closes a real hole rather than a theoretical one, and the experiment that
// found it is worth recording. The extractor's own filename sanitising replaces
// separators with lookalikes — "../../etc" becomes "..⧸..⧸etc", which is safe —
// but a BARE ".." is left exactly as it is, because it contains no separator to
// replace. Rendered into the output template that produces:
//
//	album ".."  ->  /out/Someone/../Track.mp3     (escapes the album folder)
//	artist ".." ->  /A/T.mp3                      (escapes the LIBRARY)
//
// Verified against the pinned image. --windows-filenames does not fix it and
// makes the artist case worse.
//
// SynoDL sanitises the ONE value it supplies (the group name, SanitizeName).
// It cannot sanitise these: they never pass through the server at all. So the
// guard has to travel with the command, as a constant containing no user input
// — the same property the --exec snippets rely on.
func dotOnlyGuardArgs() []string {
	out := make([]string, 0, len(directoryFields)*2)
	for _, f := range directoryFields {
		out = append(out, "--replace-in-metadata", f+" "+dotOnlyPattern+" _")
	}
	return out
}

// Options is everything needed to build a worker command.
type Options struct {
	Mode   Mode
	Target Target
	// OutDir is where the single mounted library appears inside the pod.
	OutDir string
	// MinDurationSeconds excludes clips from bulk runs. It is a LOWER bound
	// only: a long compilation is legitimate content.
	MinDurationSeconds int
	// GroupName is the playlist or channel this item came from, ALREADY
	// sanitised by SanitizeName (FR-038a/b). Empty for a directly submitted
	// link, which has no group.
	//
	// It exists because an expanded item is fetched with --no-playlist and
	// therefore has no playlist_title of its own — so without this, expanding a
	// playlist would lose the very naming that made the playlist worth keeping
	// together (FR-037).
	GroupName string
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

	// An expanded item carries its group's name as a literal album, because the
	// extractor cannot know it: --no-playlist means playlist_title is empty.
	//
	// Passed as a DISCRETE argv element and never concatenated, like the URL —
	// and already sanitised, because the same value becomes a folder inside the
	// mounted library and discrete-argv passing does nothing about a path
	// (FR-038, FR-038a).
	album := tmplAlbum
	if o.GroupName != "" {
		album = SanitizeName(o.GroupName)
	}

	args := append([]string{}, dotOnlyGuardArgs()...)
	args = append(args, []string{
		// Say what you are doing, in a format WE defined (spec 0013). --newline
		// is load-bearing: without it the extractor rewrites one line with
		// carriage returns, which is right for a terminal and unreadable for
		// anything tailing a log. Together these turn "parse the extractor's
		// human output" — which changes between releases — into "read back the
		// fields we asked for".
		"--newline",
		"--progress-template", ProgressTemplate,
		// Tags, not folders, are what a media server shelves by. Without these
		// two the album and album-artist tags come out EMPTY and every track
		// lands under "[Unknown Album]" despite a correct folder.
		"--parse-metadata", album + ":%(meta_album)s",
		"--parse-metadata", tmplArtist + ":%(meta_album_artist)s",
		"--embed-metadata",
		"--embed-thumbnail",
		"--convert-thumbnails", "jpg",
		// ".*-orig" is a regex, and it resolves the source's ORIGINAL-language
		// caption track without a metadata pre-pass.
		"--write-auto-subs",
		"--sub-langs", ".*-orig",
		"-P", outDir,
		// The output template and the album metadata template stay THE SAME
		// EXPRESSION — that identity is what keeps a track from being foldered
		// as one thing and tagged as another, and it has to survive the group
		// name being spliced in.
		"-o", tmplArtist + "/" + album + "/" + tmplTitle + ".%(ext)s",
	}...)

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

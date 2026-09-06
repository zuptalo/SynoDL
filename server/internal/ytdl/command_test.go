package ytdl

import (
	"strings"
	"testing"
)

func argsFor(t *testing.T, raw string, mode Mode) []string {
	t.Helper()
	tgt, err := Classify(raw)
	if err != nil {
		t.Fatalf("Classify(%q): %v", raw, err)
	}
	return Args(Options{Mode: mode, Target: tgt, OutDir: "/out", MinDurationSeconds: 90})
}

func has(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// value returns the argument following flag, which is how yt-dlp takes values.
func value(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func values(args []string, flag string) []string {
	var out []string
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

// FR-009: the album tag and the album folder must be incapable of disagreeing.
// The only way to guarantee that is for the two templates to be the same
// expression, so assert byte equality rather than "both look right".
func TestArgs_OutputAndAlbumTemplatesAreIdentical(t *testing.T) {
	for _, mode := range []Mode{ModeMusic, ModeMusicVideo} {
		args := argsFor(t, "https://youtu.be/abc", mode)
		out := value(args, "-o")
		albumSegment := strings.SplitN(out, "/", 3)
		if len(albumSegment) < 3 {
			t.Fatalf("[%s] -o template %q does not have three path segments", mode, out)
		}
		var albumParse string
		for _, pm := range values(args, "--parse-metadata") {
			if strings.HasSuffix(pm, ":%(meta_album)s") {
				albumParse = strings.TrimSuffix(pm, ":%(meta_album)s")
			}
		}
		if albumParse == "" {
			t.Fatalf("[%s] no --parse-metadata writing meta_album (FR-009)", mode)
		}
		if albumParse != albumSegment[1] {
			t.Errorf("[%s] album template drift: -o uses %q, --parse-metadata uses %q", mode, albumSegment[1], albumParse)
		}
	}
}

// research.md §5: adding playlist_title to the album chain tags every track of
// a channel run with the channel's tab name. It must never appear.
func TestArgs_NeverUsesPlaylistTitleAsAlbum(t *testing.T) {
	for _, raw := range []string{"https://youtu.be/abc", "https://youtube.com/playlist?list=PL1", "https://youtube.com/@x"} {
		for _, mode := range []Mode{ModeMusic, ModeMusicVideo} {
			for _, pm := range values(argsFor(t, raw, mode), "--parse-metadata") {
				if strings.Contains(pm, "playlist_title") {
					t.Errorf("%s/%s: --parse-metadata %q must not use playlist_title", raw, mode, pm)
				}
			}
		}
	}
}

func TestArgs_SharedRecipe(t *testing.T) {
	for _, mode := range []Mode{ModeMusic, ModeMusicVideo} {
		args := argsFor(t, "https://youtu.be/abc", mode)

		// FR-007: artist / album-or-Singles / title
		out := value(args, "-o")
		if want := "%(artist,uploader)s/%(album|Singles)s/%(track,title)s.%(ext)s"; out != want {
			t.Errorf("[%s] -o = %q, want %q", mode, out, want)
		}
		// FR-008: the title segment is verbatim — no restrict/replace of it.
		if has(args, "--restrict-filenames") {
			t.Errorf("[%s] --restrict-filenames would mangle the verbatim title (FR-008)", mode)
		}
		for _, r := range values(args, "--replace-in-metadata") {
			if strings.HasPrefix(r, "title") {
				t.Errorf("[%s] --replace-in-metadata %q rewrites the title (FR-008)", mode, r)
			}
		}
		// FR-009
		if !has(args, "--embed-metadata") {
			t.Errorf("[%s] missing --embed-metadata (FR-009)", mode)
		}
		var sawAlbumArtist bool
		for _, pm := range values(args, "--parse-metadata") {
			if strings.HasSuffix(pm, ":%(meta_album_artist)s") {
				sawAlbumArtist = true
			}
		}
		if !sawAlbumArtist {
			t.Errorf("[%s] missing meta_album_artist parse-metadata (FR-009)", mode)
		}
		// FR-010
		if !has(args, "--embed-thumbnail") || value(args, "--convert-thumbnails") != "jpg" {
			t.Errorf("[%s] missing cover art flags (FR-010)", mode)
		}
		// FR-011: original-language captions
		if !has(args, "--write-auto-subs") || value(args, "--sub-langs") != ".*-orig" {
			t.Errorf("[%s] missing original-language caption flags (FR-011)", mode)
		}
		if value(args, "-P") != "/out" {
			t.Errorf("[%s] -P = %q, want /out", mode, value(args, "-P"))
		}
	}
}

func TestArgs_MusicMode(t *testing.T) {
	args := argsFor(t, "https://youtu.be/abc", ModeMusic)
	if !has(args, "-x") || value(args, "--audio-format") != "mp3" || value(args, "--audio-quality") != "0" {
		t.Errorf("music mode must extract best-quality mp3, got %v", args)
	}
	if value(args, "--convert-subs") != "lrc" {
		t.Errorf("music mode wants lrc lyrics (FR-011), got %q", value(args, "--convert-subs"))
	}
	if has(args, "--merge-output-format") {
		t.Error("music mode must not merge video formats")
	}
}

// research.md §2: progressive-only is not viable (360p ceiling AND intermittent),
// and merging is a lossless stream copy rather than a conversion.
func TestArgs_MusicVideoMode(t *testing.T) {
	args := argsFor(t, "https://youtu.be/abc", ModeMusicVideo)
	f := value(args, "-f")
	if !strings.HasPrefix(f, "bv*[vcodec^=avc1]+ba[acodec^=mp4a]") {
		t.Errorf("-f must prefer avc1+mp4a for direct play, got %q", f)
	}
	if !strings.Contains(f, "/bv*+ba/b") {
		t.Errorf("-f must fall back to any video+audio then any muxed, got %q", f)
	}
	if value(args, "--merge-output-format") != "mp4" {
		t.Error("music video must merge into mp4")
	}
	if value(args, "--convert-subs") != "srt" {
		t.Error("music video wants srt subtitles")
	}
	if has(args, "-x") {
		t.Error("music video must not extract audio")
	}
}

func TestArgs_ScopeFlags(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		args := argsFor(t, "https://youtu.be/abc", ModeMusic)
		if !has(args, "--no-playlist") {
			t.Error("single scope must pass --no-playlist")
		}
		if has(args, "--download-archive") {
			t.Error("single scope must not use an archive")
		}
		if has(args, "--match-filter") {
			t.Error("single scope must not apply the duration filter: an explicit link is an explicit choice")
		}
	})

	for _, raw := range []string{"https://youtube.com/playlist?list=PL1", "https://youtube.com/@x"} {
		t.Run(raw, func(t *testing.T) {
			args := argsFor(t, raw, ModeMusic)
			// FR-013: lower bound only. A long compilation is legitimate content.
			mf := value(args, "--match-filter")
			if mf != "duration >= 90" {
				t.Errorf("--match-filter = %q, want %q", mf, "duration >= 90")
			}
			if strings.Contains(mf, "<") {
				t.Errorf("--match-filter %q must not bound duration from above (FR-013)", mf)
			}
			// FR-015: archive lives inside the target library, so audio and video
			// runs of the same item do not suppress each other.
			if arc := value(args, "--download-archive"); !strings.HasPrefix(arc, "/out/") {
				t.Errorf("--download-archive = %q, want a path inside /out (FR-015)", arc)
			}
			// FR-016: one dead entry must not fail the whole run.
			if !has(args, "-i") {
				t.Errorf("bulk scope must pass -i so an unavailable item is skipped (FR-016)")
			}
		})
	}

	t.Run("channel is polite", func(t *testing.T) {
		args := argsFor(t, "https://youtube.com/@x", ModeMusic)
		if value(args, "--sleep-requests") == "" || value(args, "--sleep-interval") == "" {
			t.Error("channel scope should pace its requests")
		}
	})
}

// Constitution v2.1.0: a user-supplied URL MUST reach the worker as a discrete
// argv element and MUST NEVER be interpolated into a shell string. This is the
// test that keeps that true as the recipe evolves.
func TestArgs_URLIsAnIsolatedArgvElement(t *testing.T) {
	hostile := []string{
		"https://youtu.be/abc",
		"https://youtu.be/abc?si=x;+rm+-rf+/",
		"https://youtu.be/abc?si=`id`",
		"https://youtu.be/abc?si=$(id)",
		"https://youtu.be/abc?si=%27%20OR%201=1",
		"https://youtu.be/abc?si=a\"b",
		"https://youtu.be/abc?si=--exec",
	}
	for _, raw := range hostile {
		tgt, err := Classify(raw)
		if err != nil {
			continue // rejected outright is also a fine outcome
		}
		args := Args(Options{Mode: ModeMusic, Target: tgt, OutDir: "/out", MinDurationSeconds: 90})

		var exact int
		for _, a := range args {
			if a == tgt.URL {
				exact++
				continue
			}
			if strings.Contains(a, tgt.URL) {
				t.Errorf("%q: URL embedded inside another argument %q", raw, a)
			}
		}
		if exact != 1 {
			t.Errorf("%q: URL appears as an exact argument %d times, want exactly 1", raw, exact)
		}
		// The URL must be last, and preceded by the end-of-options marker so a
		// URL that somehow began with "-" could never be read as a flag.
		if args[len(args)-1] != tgt.URL {
			t.Errorf("%q: URL is not the final argument", raw)
		}
		if args[len(args)-2] != "--" {
			t.Errorf("%q: URL is not guarded by an end-of-options marker", raw)
		}
	}
}

// The only shell in the whole command is our own constant --exec snippet. It
// must never contain any part of the user's input.
func TestArgs_ExecSnippetIsConstantAndUserFree(t *testing.T) {
	tgt, err := Classify("https://youtu.be/UNIQUEID12345")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []Mode{ModeMusic, ModeMusicVideo} {
		args := Args(Options{Mode: mode, Target: tgt, OutDir: "/out", MinDurationSeconds: 90})
		execs := values(args, "--exec")
		if len(execs) != 1 {
			t.Fatalf("[%s] want exactly one --exec, got %d", mode, len(execs))
		}
		if strings.Contains(execs[0], "UNIQUEID12345") || strings.Contains(execs[0], tgt.URL) {
			t.Errorf("[%s] --exec snippet contains user input: %q", mode, execs[0])
		}
		if !strings.HasPrefix(execs[0], "after_move:") {
			t.Errorf("[%s] --exec must run after_move, got %q", mode, execs[0])
		}
	}
}

package ytdl

import (
	"regexp"
	"strconv"
	"strings"
)

// Reading what a worker says about itself (spec 0013).
//
// Spec 0012 reported no progress, and said why: "the worker has no channel back
// to us". It does — its own output — and the whole design of this file is about
// making that channel something SynoDL DEFINED rather than something it guessed.
//
// The worker is run with an explicit --progress-template, so the lines below are
// a format we specified. Anything else on stdout is the extractor's own chatter,
// whose shape changes between releases, and it is ignored rather than parsed
// hopefully. That is what the sentinel is for: a line SynoDL acts on is a line
// SynoDL asked for.

// ProgressSentinel marks a line as ours. It is deliberately unlike anything the
// extractor prints on its own.
const ProgressSentinel = "[synodl]"

// ProgressTemplate is what the worker is told to print, once per update.
//
// A CONSTANT authored here, containing no user input — the same property the
// --exec snippets rely on. The field names are ours; the values come from the
// extractor's own progress dictionary.
const ProgressTemplate = ProgressSentinel + " status=%(progress.status)s" +
	" downloaded=%(progress.downloaded_bytes)s" +
	" total=%(progress.total_bytes,progress.total_bytes_estimate)s" +
	" stream=%(info.__stream_index)s of=%(info.__stream_count)s"

// ProgressReading is one parsed line.
type ProgressReading struct {
	Status     string
	Downloaded int64
	Total      int64
	// Stream and Streams describe WHICH pass this is, for a fetch that combines
	// separate picture and sound streams. Both zero means a single stream.
	Stream  int
	Streams int
}

// Fraction converts a reading into progress over the WHOLE download, 0–1.
//
// The music-video format selector deliberately asks for separate picture and
// sound streams, because YouTube's only progressive format is 640x360 and is
// served intermittently. That means the extractor reports two independent runs
// of 0→100%, and rendering them raw would show a bar that restarts half way
// through — which FR-012 forbids. Each stream therefore occupies its own slice
// of the whole.
//
// Returns false when nothing is known, so the caller can show no percentage
// rather than a confident zero (FR-013).
func (r ProgressReading) Fraction() (float64, bool) {
	if r.Total <= 0 || r.Downloaded < 0 {
		return 0, false
	}
	within := float64(r.Downloaded) / float64(r.Total)
	if within > 1 {
		within = 1
	}

	streams, stream := r.Streams, r.Stream
	if streams <= 1 {
		return within, true
	}
	if stream < 1 {
		stream = 1
	}
	if stream > streams {
		stream = streams
	}
	// Stream n of N covers [(n-1)/N, n/N].
	share := 1 / float64(streams)
	return (float64(stream-1) + within) * share, true
}

// progressLine matches only lines we asked for: the sentinel, then a space.
var progressLine = regexp.MustCompile(`^` + regexp.QuoteMeta(ProgressSentinel) + ` (.+)$`)

// ParseProgress reads one line, reporting false for anything that is not ours.
func ParseProgress(line string) (ProgressReading, bool) {
	m := progressLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return ProgressReading{}, false
	}
	r := ProgressReading{}
	for _, field := range strings.Fields(m[1]) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		// An unparseable value is left at zero rather than discarding the line.
		// "NA" for an unknown total is an ordinary reading from a live stream,
		// and losing the whole line over it would lose the status too.
		switch k {
		case "status":
			r.Status = v
		case "downloaded":
			r.Downloaded, _ = strconv.ParseInt(v, 10, 64)
		case "total":
			r.Total, _ = strconv.ParseInt(v, 10, 64)
		case "stream":
			r.Stream, _ = strconv.Atoi(v)
		case "of":
			r.Streams, _ = strconv.Atoi(v)
		}
	}
	return r, true
}

// companionLine matches the extractor's note that it wrote a subtitle or lyrics
// file, and captures the language infix from the name.
//
// The extractor always names a companion file <base>.<lang>.<ext> and offers no
// way to omit the language infix — the same fact the --exec rename snippets in
// command.go exist to work around. Here that infix is the useful part: it is
// how the language gets reported without the server ever seeing the file.
// The path is matched with `.*` rather than `\S*` on purpose: a track's own
// title becomes its filename verbatim, and titles contain spaces.
var companionLine = regexp.MustCompile(`Writing video subtitles to:\s+.*\.([A-Za-z]{2,3}(?:-[A-Za-z0-9]+)?)(?:-orig)?\.(?:lrc|srt|vtt)\s*$`)

// ParseCompanion reports the language of a companion file the worker just wrote.
func ParseCompanion(line string) (string, bool) {
	m := companionLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return "", false
	}
	// "-orig" is a marker meaning original-language, not part of the language.
	return strings.TrimSuffix(m[1], "-orig"), true
}

// OutputReading is everything one read of a worker's output yielded.
type OutputReading struct {
	HasProgress bool
	Latest      ProgressReading
	HasLyrics   bool
	LyricsLang  string
}

// ScanOutput reads a block of worker output.
//
// Takes the LAST progress line rather than the first: a log tail is a window
// over recent output, and the most recent reading is the current one.
func ScanOutput(raw []byte) OutputReading {
	var out OutputReading
	for _, line := range strings.Split(string(raw), "\n") {
		if r, ok := ParseProgress(line); ok {
			out.HasProgress, out.Latest = true, r
			continue
		}
		if lang, ok := ParseCompanion(line); ok {
			out.HasLyrics, out.LyricsLang = true, lang
		}
	}
	return out
}

// ProgressTracker clamps a series of readings so the value only ever advances.
//
// Needed because readings can arrive out of order: the reconciler re-reads a
// window of the log each cycle, and the last line of one stream can be seen
// after the first line of the next. Clamping happens on the SERVER rather than
// in the client, because several viewers read the same held value and the value
// itself has to already be right (FR-012).
type ProgressTracker struct{ high float64 }

// Observe records a reading and returns the value to show.
func (t *ProgressTracker) Observe(f float64) float64 {
	if f > t.high {
		t.high = f
	}
	if t.high > 1 {
		t.high = 1
	}
	return t.high
}

// Value reports the highest reading seen so far.
func (t *ProgressTracker) Value() float64 { return t.high }

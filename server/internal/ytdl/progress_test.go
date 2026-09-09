package ytdl

import "testing"

// Parsing a worker's own output (spec 0013).
//
// The reason this is worth testing hard: it is the one place where text from a
// third-party program crosses into SynoDL's model of what is happening. Every
// case below is either a line we asked for, or a line we must refuse to guess at.

func TestParseProgress_ReadsOurOwnLines(t *testing.T) {
	line := ProgressSentinel + " status=downloading downloaded=524288 total=1048576 stream=1 of=2"
	r, ok := ParseProgress(line)
	if !ok {
		t.Fatalf("a well-formed line was not parsed: %q", line)
	}
	if r.Status != "downloading" || r.Downloaded != 524288 || r.Total != 1048576 {
		t.Fatalf("parsed %+v, want the fields we asked the worker to print", r)
	}
	if r.Stream != 1 || r.Streams != 2 {
		t.Fatalf("stream = %d of %d, want 1 of 2", r.Stream, r.Streams)
	}
}

func TestParseProgress_IgnoresAnythingWeDidNotAskFor(t *testing.T) {
	// A worker's stdout is full of its own chatter. Acting on a line we did not
	// specify would mean guessing at a format that changes between releases —
	// which is the brittleness the sentinel exists to avoid.
	for _, line := range []string{
		"",
		"[download] Destination: /out/Queen/Singles/Bohemian Rhapsody.mp3",
		"[download]  42.3% of 5.00MiB at 1.20MiB/s ETA 00:03",
		"WARNING: unable to extract something",
		"ERROR: video unavailable",
		ProgressSentinel,                                     // marker with nothing after it
		ProgressSentinel + "extra status=x",                  // marker not followed by a space
		"prefix " + ProgressSentinel + " status=downloading", // marker not at the start
	} {
		if r, ok := ParseProgress(line); ok {
			t.Errorf("ParseProgress(%q) = %+v, want it ignored", line, r)
		}
	}
}

func TestParseProgress_ToleratesGarbageInFieldsWeAskedFor(t *testing.T) {
	// The line is ours, but a value can still be missing or unparseable — an
	// unknown total is normal for a live stream. That must not lose the line.
	r, ok := ParseProgress(ProgressSentinel + " status=downloading downloaded=abc total=NA")
	if !ok {
		t.Fatal("a line of ours with unparseable numbers should still be recognised")
	}
	if r.Downloaded != 0 || r.Total != 0 {
		t.Fatalf("parsed %+v, want unparseable numbers treated as unknown", r)
	}
	if _, known := r.Fraction(); known {
		t.Error("fraction should be unknown when the numbers are")
	}
}

func TestFraction_NeverExceedsOrRestarts(t *testing.T) {
	// FR-012. The music-video path fetches picture and sound as SEPARATE
	// streams, so the worker reports two independent runs of 0→100%. Rendered
	// raw, the bar would visibly restart half way through.
	first, ok := ProgressReading{Status: "downloading", Downloaded: 100, Total: 100, Stream: 1, Streams: 2}.Fraction()
	if !ok || first > 0.51 || first < 0.49 {
		t.Fatalf("end of stream 1 of 2 = %v, want about half of the whole", first)
	}
	second, ok := ProgressReading{Status: "downloading", Downloaded: 50, Total: 100, Stream: 2, Streams: 2}.Fraction()
	if !ok {
		t.Fatal("second stream should report")
	}
	if second < first {
		t.Fatalf("stream 2 at 50%% reported %v, which is BEHIND stream 1's %v — the bar would go backwards", second, first)
	}
	full, _ := ProgressReading{Status: "downloading", Downloaded: 100, Total: 100, Stream: 2, Streams: 2}.Fraction()
	if full > 1 {
		t.Fatalf("fraction = %v, want it never to exceed completion", full)
	}
}

func TestFraction_SingleStreamIsTheWholeThing(t *testing.T) {
	// The audio path is one stream, so its own percentage IS the download's.
	got, ok := ProgressReading{Status: "downloading", Downloaded: 25, Total: 100}.Fraction()
	if !ok || got < 0.24 || got > 0.26 {
		t.Fatalf("fraction = %v (%v), want about a quarter", got, ok)
	}
}

func TestProgressTracker_IsMonotonic(t *testing.T) {
	// Lines can arrive out of order — a tail re-reads the same window, and the
	// last line of a finished stream can be read after the first of the next.
	// The displayed value must only ever advance (FR-012).
	var tr ProgressTracker
	seq := []float64{0.1, 0.4, 0.3, 0.42, 0.2, 0.9}
	var last float64
	for _, v := range seq {
		got := tr.Observe(v)
		if got < last {
			t.Fatalf("observing %v moved progress from %v back to %v", v, last, got)
		}
		last = got
	}
	if last < 0.89 {
		t.Fatalf("final = %v, want it to have reached the highest value seen", last)
	}
}

func TestParseCompanion_FindsLyricsAndItsLanguage(t *testing.T) {
	// FR-011. The worker says what it wrote; this is the only place that fact
	// exists, because the server never mounts the media library.
	for _, tc := range []struct {
		line, lang string
		want       bool
	}{
		{`[info] Writing video subtitles to: /out/Queen/Singles/Bohemian Rhapsody.en-orig.lrc`, "en", true},
		{`[info] Writing video subtitles to: /out/A/B/track.fa-orig.lrc`, "fa", true},
		{`[info] Writing video subtitles to: /out/A/B/track.en.srt`, "en", true},
		{`[info] Writing video subtitles to: /out/A/B/track.pt-BR-orig.srt`, "pt-BR", true},
		{`[download] Destination: /out/A/B/track.mp3`, "", false},
		{`[info] Writing thumbnail to: /out/A/B/track.jpg`, "", false},
		{``, "", false},
	} {
		lang, ok := ParseCompanion(tc.line)
		if ok != tc.want || lang != tc.lang {
			t.Errorf("ParseCompanion(%q) = (%q, %v), want (%q, %v)", tc.line, lang, ok, tc.lang, tc.want)
		}
	}
}

func TestScanOutput_ReadsAWholeLogBlock(t *testing.T) {
	out := `[youtube] Extracting URL: https://youtu.be/abc
` + ProgressSentinel + ` status=downloading downloaded=10 total=100
` + ProgressSentinel + ` status=downloading downloaded=90 total=100
[info] Writing video subtitles to: /out/Queen/Singles/Bohemian Rhapsody.en-orig.lrc
[download] 100% of 5.00MiB
`
	r := ScanOutput([]byte(out))
	if !r.HasProgress {
		t.Fatal("progress lines were present and should have been read")
	}
	if f, ok := r.Latest.Fraction(); !ok || f < 0.89 {
		t.Fatalf("latest fraction = %v (%v), want the LAST reading, not the first", f, ok)
	}
	if !r.HasLyrics || r.LyricsLang != "en" {
		t.Fatalf("lyrics = (%v, %q), want them found with their language", r.HasLyrics, r.LyricsLang)
	}
}

func TestScanOutput_EmptyOrIrrelevantSaysNothing(t *testing.T) {
	// FR-013: nothing known must stay nothing known. An invented zero would
	// render as a download stuck at the start.
	for _, out := range []string{"", "[youtube] Extracting URL\nWARNING: nope\n"} {
		r := ScanOutput([]byte(out))
		if r.HasProgress || r.HasLyrics {
			t.Errorf("ScanOutput(%q) claimed to know something: %+v", out, r)
		}
	}
}

// What the AUDIO path actually emits (verified against the pinned image, T095).
//
// A single-stream download has no stream index, and the extractor renders an
// absent field as the literal "NA" rather than omitting it. That is the real
// shape of most lines this parser will ever see, so it is worth pinning
// exactly rather than approximating with a synthetic one.
func TestParseProgress_AudioPathRendersNAForStreamFields(t *testing.T) {
	line := ProgressSentinel + " status=downloading downloaded=524288 total=1048576 stream=NA of=NA"
	r, ok := ParseProgress(line)
	if !ok {
		t.Fatalf("the audio path's own line was not parsed: %q", line)
	}
	if r.Status != "downloading" || r.Downloaded != 524288 || r.Total != 1048576 {
		t.Fatalf("parsed %+v, want the byte counts read", r)
	}
	// "NA" is not a number, so both stay zero — which Fraction reads as a single
	// stream, and the whole download is that one stream.
	if r.Stream != 0 || r.Streams != 0 {
		t.Fatalf("stream=%d of=%d, want NA treated as absent", r.Stream, r.Streams)
	}
	f, known := r.Fraction()
	if !known || f < 0.49 || f > 0.51 {
		t.Fatalf("fraction = %v (%v), want half — a single stream IS the whole download", f, known)
	}
}

// The estimate fallback in the template, which is what a live or
// unknown-length item reports.
func TestParseProgress_TotalMayBeAnEstimate(t *testing.T) {
	r, ok := ParseProgress(ProgressSentinel + " status=downloading downloaded=1000 total=9000 stream=NA of=NA")
	if !ok {
		t.Fatal("not parsed")
	}
	if f, known := r.Fraction(); !known || f < 0.11 || f > 0.12 {
		t.Fatalf("fraction = %v (%v), want about an ninth", f, known)
	}
}

// Post-processing (extracting to mp3) emits no download progress of its own, so
// the last line before it says "finished". The bar sitting full while the file
// is converted is honest: the DOWNLOAD is done.
func TestParseProgress_FinishedStatusIsFullNotUnknown(t *testing.T) {
	r, ok := ParseProgress(ProgressSentinel + " status=finished downloaded=1048576 total=1048576 stream=NA of=NA")
	if !ok {
		t.Fatal("not parsed")
	}
	if r.Status != "finished" {
		t.Fatalf("status = %q", r.Status)
	}
	if f, known := r.Fraction(); !known || f < 0.99 {
		t.Fatalf("fraction = %v (%v), want it full", f, known)
	}
}

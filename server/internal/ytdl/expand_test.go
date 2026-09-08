package ytdl

import (
	"strings"
	"testing"
)

func entryLine(id, title string) string {
	return ExpandSentinel + " id=" + id + " uploader=Lo-fi Beats title=" + title
}

func TestExpandArgs_ListsWithoutDownloading(t *testing.T) {
	args := ExpandArgs(Target{URL: "https://www.youtube.com/@lofi/videos", Scope: ScopeChannel})

	for _, want := range []string{"--flat-playlist", "--skip-download", "--print"} {
		if !contains(args, want) {
			t.Errorf("%s missing — an enumeration must list without fetching anything", want)
		}
	}
	if contains(args, "-x") || contains(args, "--embed-thumbnail") || contains(args, "-P") {
		t.Error("enumeration args carry download flags; this worker writes nothing")
	}
	// Same argv rule as every other worker command.
	if args[len(args)-1] != "https://www.youtube.com/@lofi/videos" || args[len(args)-2] != "--" {
		t.Fatalf("args do not end with `-- <url>`: %v", args)
	}
	var n int
	for _, a := range args {
		if a == "https://www.youtube.com/@lofi/videos" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("URL appears %d times, want exactly once", n)
	}
}

func TestParseEntries_ReadsOurOwnLines(t *testing.T) {
	raw := strings.Join([]string{
		"[youtube:tab] Extracting URL: https://www.youtube.com/@lofi/videos",
		entryLine("aaaaaaaaaaa", "Midnight Study"),
		entryLine("bbbbbbbbbbb", "Rainy Window"),
		"[download] Finished downloading playlist",
	}, "\n")

	got := ParseEntries([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2: %+v", len(got), got)
	}
	if got[0].ID != "aaaaaaaaaaa" || got[0].Title != "Midnight Study" {
		t.Fatalf("first entry = %+v", got[0])
	}
	if got[0].URL != "https://www.youtube.com/watch?v=aaaaaaaaaaa" {
		t.Fatalf("URL = %q, want a canonical watch link rebuilt from the id", got[0].URL)
	}
}

func TestParseEntries_CarriesTheUploader(t *testing.T) {
	// A channel publishes no oEmbed document, so this is the only place its real
	// name can be learned (FR-036).
	got := ParseEntries([]byte(entryLine("abc", "Midnight Study")))
	if len(got) != 1 || got[0].Uploader != "Lo-fi Beats" {
		t.Fatalf("got %+v, want the uploader carried through", got)
	}
}

func TestParseEntries_TitlesMayContainSpaces(t *testing.T) {
	// Titles with spaces are the norm, not the exception — parsing the title as
	// a whitespace-delimited field would truncate almost every one of them.
	got := ParseEntries([]byte(entryLine("abc", "A Long Title With Many Spaces")))
	if len(got) != 1 || got[0].Title != "A Long Title With Many Spaces" {
		t.Fatalf("got %+v, want the whole title", got)
	}
}

func TestParseEntries_IgnoresAnythingWeDidNotAskFor(t *testing.T) {
	for _, line := range []string{
		"",
		"[youtube:tab] Downloading page 3",
		"WARNING: unable to extract",
		"ERROR: This playlist is private",
		ExpandSentinel,
		ExpandSentinel + "id=abc", // no space after the marker
		"prefix " + ExpandSentinel + " id=abc title=x",
	} {
		if got := ParseEntries([]byte(line)); len(got) != 0 {
			t.Errorf("ParseEntries(%q) returned %+v, want nothing", line, got)
		}
	}
}

// FR-016a. Enumeration output is external input describing an external site,
// and what it produces is about to be handed to another worker. Every derived
// link goes through the SAME host allowlist a typed link does.
func TestParseEntries_DerivedLinksAreAllowlisted(t *testing.T) {
	hostile := []string{
		entryLine("../../etc/passwd", "traversal"),
		entryLine("abc?v=x&next=https://evil.example/", "smuggled query"),
		entryLine("https://evil.example/watch?v=abc", "a whole URL as an id"),
		entryLine("", "no id at all"),
		entryLine("  ", "blank id"),
	}
	for _, line := range hostile {
		for _, e := range ParseEntries([]byte(line)) {
			if !strings.HasPrefix(e.URL, "https://www.youtube.com/watch?v=") &&
				!strings.HasPrefix(e.URL, "https://youtu.be/") &&
				!strings.HasPrefix(e.URL, "https://music.youtube.com/") {
				t.Errorf("line %q produced %q, which is off the allowlist", line, e.URL)
			}
			if strings.Contains(e.URL, "evil.example") {
				t.Errorf("line %q smuggled a hostile host into %q", line, e.URL)
			}
		}
	}
}

func TestParseEntries_DropsEntriesWithNoID(t *testing.T) {
	// An entry SynoDL cannot identify is one it could neither de-duplicate nor
	// retry, so it is dropped rather than guessed at (FR-016b).
	raw := strings.Join([]string{
		ExpandSentinel + " title=No id here",
		entryLine("goodid", "Fine"),
	}, "\n")
	got := ParseEntries([]byte(raw))
	if len(got) != 1 || got[0].ID != "goodid" {
		t.Fatalf("got %+v, want only the identifiable entry", got)
	}
}

func TestParseEntries_CollapsesDuplicates(t *testing.T) {
	// A channel can list the same video across tabs. Queueing it twice would
	// race two workers onto one file — the failure the duplicate rule exists for.
	raw := strings.Join([]string{
		entryLine("same", "Once"),
		entryLine("same", "Twice"),
		entryLine("other", "Different"),
	}, "\n")
	got := ParseEntries([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want duplicates collapsed: %+v", len(got), got)
	}
}

// FR-017: there is no ceiling on expansion, and nothing in this code may quietly
// introduce one. That was an explicit product decision and it is otherwise
// guarded by nothing at all.
func TestParseEntries_HasNoCeiling(t *testing.T) {
	const many = 5000
	var b strings.Builder
	for i := 0; i < many; i++ {
		b.WriteString(entryLine(idFor(i), "Track"))
		b.WriteByte('\n')
	}
	got := ParseEntries([]byte(b.String()))
	if len(got) != many {
		t.Fatalf("parsed %d of %d entries — expansion must not be capped or silently truncated", len(got), many)
	}
}

func idFor(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, 0, 11)
	for i := 0; i < 11; i++ {
		out = append(out, alphabet[(n+i*7)%len(alphabet)])
		n /= len(alphabet)
	}
	return string(out)
}

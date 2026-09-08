package ytdl

import (
	"net/url"
	"strings"
)

// Turning a playlist or channel into its individual items (spec 0013, US6).
//
// Spec 0012 fetched a playlist or channel as ONE bulk request. That works, and
// it cannot say which item is downloading, cannot report which item failed, and
// cannot be retried at item granularity — so a channel was all-or-nothing in a
// way nothing else in the app is.
//
// SynoDL cannot enumerate the source itself: the server has no extractor, and
// putting one in the server image would break "the download never runs inside
// the server process" (0012 FR-005) as well as making the server image no longer
// just a Go binary. So enumeration is its own short-lived worker — the same
// model as a download, doing a different job — and this file is the two halves
// SynoDL owns: what to ask it, and how to read what it says.

// ExpandSentinel marks an entry line as ours, exactly as ProgressSentinel does
// for progress. A line SynoDL acts on is a line SynoDL asked for.
const ExpandSentinel = "[synodl-entry]"

// ExpandTemplate is the per-entry line the worker is told to print.
//
// A constant containing no user input. `id` is the item's stable identity and
// `title` is what to show while it waits; anything else can be learned later,
// and asking for more here would slow an enumeration that may cover thousands of
// items.
const ExpandTemplate = ExpandSentinel + " id=%(id)s title=%(title)s"

// ExpandArgs builds the argv for an enumeration worker.
//
// --flat-playlist is the load-bearing flag: it lists what a playlist or channel
// contains WITHOUT resolving each entry, which is the difference between reading
// a large channel in seconds and re-extracting every video in it. Downloading is
// explicitly disabled — this worker writes nothing and mounts no library.
//
// The URL is the final element after the end-of-options marker, appears exactly
// once, and is never concatenated into anything — the same rule as every other
// worker command here.
func ExpandArgs(t Target) []string {
	return []string{
		"--flat-playlist",
		"--skip-download",
		"--no-warnings",
		// One unavailable entry must not fail the enumeration: a channel with a
		// single private video is entirely ordinary.
		"-i",
		"--print", ExpandTemplate,
		"--", t.URL,
	}
}

// Entry is one item an enumeration found.
type Entry struct {
	// ID is the source's own identifier for the item.
	ID string
	// Title is what to show while it waits its turn. Often empty, and that is
	// fine — the row falls back to the link, as it already does elsewhere.
	Title string
	// URL is the item's address, rebuilt by SynoDL from the id rather than taken
	// from the worker. See ParseEntries.
	URL string
}

// ParseEntries reads a block of enumeration output.
//
// Two things here are deliberate and load-bearing:
//
//   - The URL is REBUILT from the id, never taken from the worker's output. What
//     comes back from an enumeration is external input describing an external
//     site, and the URL it produces is about to be handed to another worker. So
//     SynoDL constructs a canonical watch URL from the id and then puts it
//     through Classify — the same host allowlist a user-typed link passes
//     (FR-016a). An entry pointing anywhere else simply does not survive.
//   - Entries with no id are dropped rather than guessed at. An entry SynoDL
//     cannot identify is one it could neither de-duplicate nor retry.
//
// Duplicates are collapsed: a channel can list the same video twice across
// tabs, and queueing it twice would race two workers onto one file.
func ParseEntries(raw []byte) []Entry {
	var out []Entry
	seen := map[string]bool{}

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, ExpandSentinel+" ") {
			continue
		}
		rest := strings.TrimPrefix(line, ExpandSentinel+" ")

		id, title := parseEntryFields(rest)
		if !validEntryID(id) || seen[id] {
			continue
		}

		// Rebuild, then re-validate. Nothing from the worker becomes a URL.
		target, err := Classify("https://www.youtube.com/watch?v=" + url.QueryEscape(id))
		if err != nil || target.Scope != ScopeSingle {
			continue
		}

		seen[id] = true
		out = append(out, Entry{ID: id, Title: strings.TrimSpace(title), URL: target.URL})
	}
	return out
}

// validEntryID accepts only something shaped like an item id.
//
// The allowlist below is what stops a malformed entry becoming a download at
// all. Percent-encoding an arbitrary string into a query value is SAFE — the
// host stays youtube.com and the argv rule still holds — but it is not
// harmless: it produces a queued download that can never resolve, and on a
// large channel it would produce a great many of them. An id that is not an id
// is not an item, so it is dropped here rather than fetched and failed later.
func validEntryID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// parseEntryFields splits `id=<id> title=<the rest>`.
//
// Title is taken as everything after `title=` rather than as a whitespace-
// delimited field, because titles contain spaces — most of them do.
func parseEntryFields(s string) (id, title string) {
	const idKey, titleKey = "id=", " title="
	if !strings.HasPrefix(s, idKey) {
		return "", ""
	}
	s = strings.TrimPrefix(s, idKey)
	if i := strings.Index(s, titleKey); i >= 0 {
		return s[:i], s[i+len(titleKey):]
	}
	return s, ""
}

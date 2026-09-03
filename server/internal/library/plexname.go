package library

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Naming a NEW download the way a media server expects (spec 1021).
//
// This is the write side of the same convention scripts/library_tidy.py applies
// to a library retrospectively: "Despicable Me 4 (2024)" and
// "Friends (1994)/Season 01". Keeping the two in step is the whole point — if
// they disagree, every download re-introduces the mess the tidy just removed.
//
// Only the destination FOLDER is ours to name. The file names inside come from
// the source and are left exactly as they arrive.

// alreadyParenthesised matches a name that already ends in "(1999)", so a title
// the catalog happens to give us in the right shape is not given a second pair.
var alreadyParenthesised = regexp.MustCompile(`\((?:19|20)\d{2}\)\s*$`)

// PlexName turns a raw catalog title into "Title (Year)".
//
// The sources put the year at the end, bare ("Despicable Me 4 2024") or as a
// range covering a series' run ("Friends 1994 - 2004"). A scraper keys a show on
// its FIRST air year, so a range collapses to its start. A title carrying no
// year keeps its name rather than gaining empty parentheses, and a title that IS
// a year ("1917") is left whole rather than emptied.
func PlexName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || alreadyParenthesised.MatchString(s) {
		return s
	}
	m := trailingYear.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	head := strings.TrimSpace(s[:len(s)-len(m[0])])
	if head == "" {
		return s // the year is the whole title
	}
	return head + " (" + m[1] + ")"
}

// SeasonFolder is the subfolder a season's episodes belong in. Two digits is the
// convention every scraper documents, and it keeps a listing in season order.
func SeasonFolder(n int) string {
	return fmt.Sprintf("Season %02d", n)
}

// seasonEpisode matches "S01E02", "s1.e2" and "1x05" anywhere in a name.
//
// Bounded on non-alphanumerics rather than \b, because the sources separate with
// underscores and \b treats "_" as a word character — so "X_Men_97_S02E03" would
// never match. This mirrors SE_RE in src/services/task-title.ts.
var seasonEpisode = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s(\d{1,2})[^a-z0-9]?e\d{1,3}|(\d{1,2})x\d{1,3})(?:$|[^a-z0-9])`)

// SeasonOfFiles reports the season the given files belong to.
//
// The season is read from the files themselves rather than taken from the
// client, so it can never disagree with what actually lands on disk. A set
// spanning more than one season is reported as unknown: filing half of it under
// the wrong season is worse than filing all of it directly in the show's folder,
// which a scraper still reads correctly.
func SeasonOfFiles(files []string) (int, bool) {
	season, found := 0, false
	for _, f := range files {
		// A signed link carries a query full of digits; only the name matters.
		name := path.Base(f)
		if i := strings.IndexAny(name, "?#"); i >= 0 {
			name = name[:i]
		}
		m := seasonEpisode.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if found && n != season {
			return 0, false // spans seasons: ambiguous, so say so
		}
		season, found = n, true
	}
	return season, found
}

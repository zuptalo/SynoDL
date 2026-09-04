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
var seasonEpisode = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:s(\d{1,2})[^a-z0-9]?e(\d{1,3})|(\d{1,2})x(\d{1,3}))(?:$|[^a-z0-9])`)

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
			raw = m[3] // x-form season; groups 2 and 4 are the episode numbers
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

// EpisodeOf reads the season and episode a file name declares.
//
// The season alone cannot answer "which episodes do I have" (FR-016), and the
// number was already being matched and discarded. A name that declares neither is
// reported as such rather than guessed at: FR-016b makes an unreadable name a
// season still counted as present, never an episode invented.
func EpisodeOf(name string) (season, episode int, ok bool) {
	// A signed link carries a query full of digits; only the name matters.
	base := path.Base(name)
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	m := seasonEpisode.FindStringSubmatch(base)
	if m == nil {
		return 0, 0, false
	}
	sRaw, eRaw := m[1], m[2]
	if sRaw == "" {
		sRaw, eRaw = m[3], m[4]
	}
	s, err1 := strconv.Atoi(sRaw)
	e, err2 := strconv.Atoi(eRaw)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return s, e, true
}

// seasonFolder matches the folder names a season is stored under: "Season 01",
// "Season 1", "S01", "Series 2" (the spelling some scrapers write), and the
// specials folder, which Plex names "Specials" and which is season 0.
//
// Anchored at both ends so a real title beginning with the word — "Seasons of
// Love" — is not mistaken for one, the same guard SEASON_FOLDER applies in
// src/services/task-title.ts.
var seasonFolder = regexp.MustCompile(`^(?i)\s*(?:season|series|s)\s*(\d{1,4})\s*$`)
var specialsFolder = regexp.MustCompile(`^(?i)\s*specials?\s*$`)

// SeasonOfFolder reads the season number a folder name declares.
//
// Used only as a FALLBACK: the season is taken from the episode files where they
// say, because those are what actually landed. A folder can be named anything,
// and its name is not evidence of what is inside it — the lesson FR-001a records.
func SeasonOfFolder(name string) (int, bool) {
	if specialsFolder.MatchString(name) {
		return 0, true // Plex files specials as season 0
	}
	m := seasonFolder.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

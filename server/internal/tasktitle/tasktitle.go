// Package tasktitle derives a human-readable title (and season/episode marker)
// for a download, for use in push-notification text. It is a Go port of the
// client's src/services/task-title.ts and MUST stay behaviourally in sync with
// it (same folder-title derivation, same S01E05 extraction) — the two share a
// test corpus so a divergence shows up as a failing case on one side.
package tasktitle

import (
	"fmt"
	"regexp"
	"strings"
)

// seRE matches S01E05 / s1.e5 / S01 E05 … anywhere in a name or URL, bounded on
// non-alphanumerics (or the string edges) rather than \b — the source separates
// with underscores ("X_Men_97_S01E01_…") and \b treats "_" as a word char. RE2
// has no lookahead, so the trailing boundary is consumed; that's harmless here
// because only the digits are captured. Mirrors SE_RE in task-title.ts.
var seRE = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,2})[^a-z0-9]?e(\d{1,3})(?:$|[^a-z0-9])`)

// mediaParentRE marks a folder whose parent is a media bucket, so its leaf is a
// real title rather than a generic download folder. Mirrors MEDIA_PARENT.
var mediaParentRE = regexp.MustCompile(`(?i)^(movie|movies|tv|tv-?shows?|series|anime|video|videos)$`)

// episodeOf returns the first "S01E05"-style marker found across the sources.
func episodeOf(sources ...string) string {
	for _, s := range sources {
		if s == "" {
			continue
		}
		if m := seRE.FindStringSubmatch(s); m != nil {
			return fmt.Sprintf("S%02sE%02s", pad2(m[1]), pad2(m[2]))
		}
	}
	return ""
}

// pad2 left-pads a 1-digit number to 2 digits (matching padStart(2,'0')).
func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func pathParts(destination string) []string {
	out := []string{}
	for _, p := range strings.Split(destination, "/") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// seasonFolderRE matches a season subfolder, which a series' destination now
// ends in (spec 1021): "Season 01", "Season 2024". Anchored at both ends so a
// real title beginning with the word — "Seasons of Love" — is not mistaken for
// one. Mirrors SEASON_FOLDER in task-title.ts.
var seasonFolderRE = regexp.MustCompile(`(?i)^season\s+\d{1,4}$`)

// titleEnd is the index one past the per-title folder. Normally the end of the
// path, but a leaf that is a season folder means the title is one level up.
func titleEnd(parts []string) int {
	if len(parts) >= 2 && seasonFolderRE.MatchString(parts[len(parts)-1]) {
		return len(parts) - 1
	}
	return len(parts)
}

func isMediaFolder(destination string) bool {
	parts := pathParts(destination)
	end := titleEnd(parts)
	return end >= 2 && mediaParentRE.MatchString(parts[end-2])
}

// Title returns a readable title and (when detectable) the "S01E05" episode
// marker for a task. It prefers the destination's leaf folder as the title only
// when that's clearly a media download (a media parent, or a detected episode);
// otherwise the raw name is more meaningful. Mirrors taskTitle() in
// task-title.ts.
func Title(name, destination, uri string) (title, episode string) {
	episode = episodeOf(name, uri)
	parts := pathParts(destination)
	folder := ""
	if end := titleEnd(parts); end > 0 {
		folder = parts[end-1]
	}
	useFolder := folder != "" && (episode != "" || isMediaFolder(destination))
	if useFolder {
		return folder, episode
	}
	return name, episode
}

// Display is the notification-ready string: "Title · S01E05" when an episode is
// present, otherwise just the title. Falls back to the raw name (never empty)
// when no title is derivable.
func Display(name, destination, uri string) string {
	title, episode := Title(name, destination, uri)
	if title == "" {
		title = name
	}
	if episode != "" {
		return title + " · " + episode
	}
	return title
}

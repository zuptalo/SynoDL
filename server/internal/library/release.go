package library

import (
	"path"
	"regexp"
	"strings"
	"unicode"
)

// Identifying WHICH copy of a title is on the NAS (spec 1025).
//
// Ownership used to be answered per season: if any video for season 1 was on
// disk, every season-1 download option was marked as already downloaded — the
// 1080p one, the x265 one and the other encoder's alike. Only one of them was
// the file the user actually had, so the marker said something untrue about
// exactly the options they were choosing between.
//
// The files themselves know better. SynoDL saves a download under the name it
// arrived with, and release names carry both a resolution and the group that
// produced the encode. Those two together identify a release; either one alone
// does not, because several releases of the same season share a resolution and
// differ only by who made them.

// Release is how one copy was encoded.
//
// Group is stored folded (lower case, separators removed) because it is only
// ever compared, never displayed — the file name it came from must not leave
// the server.
type Release struct {
	Resolution string
	Group      string
	// Key identifies the release by the FILE it is, rather than by tokens read out
	// of it — see ReleaseKey. Filled even when Resolution and Group are not, which
	// is the common case for a source that renames what it serves.
	Key string
}

var (
	// Episode notation, removed from a release key so every episode of one release
	// identifies it. The SEASON is kept — two seasons of the same encode are
	// different things and must not collapse onto each other.
	reEpisodeToken = regexp.MustCompile(`(?i)\bs(\d{1,2})[\. _-]?e\d{1,3}\b`)
	reXEpisode     = regexp.MustCompile(`(?i)\b(\d{1,2})x\d{1,3}\b`)
	// The resolution tokens release names actually use.
	reResolution = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|360p)\b`)
	// 4K and UHD mean 2160p; an option will say 2160p, so normalize on the way in.
	reUHD = regexp.MustCompile(`(?i)\b(4k|uhd)\b`)
	// The trailing "-GROUP" of a scene-style name, and the bracketed form some
	// releases use instead. Anchored at the end so a hyphen inside the title
	// ("WEB-DL", "Spider-Man") is never mistaken for the group separator.
	reTrailingGroup  = regexp.MustCompile(`-([A-Za-z0-9]{2,})$`)
	reBracketedGroup = regexp.MustCompile(`[\[(]([A-Za-z0-9]{2,})[\])]\s*$`)
)

// ReleaseOf reads the release a file name describes.
//
// ok is false unless BOTH halves are found. A half-identified file identifies
// nothing: marking an option because the resolution agreed is the guess that
// makes three options out of four wrong, which is the whole bug.
func ReleaseOf(name string) (Release, bool) {
	base := strings.TrimSpace(path.Base(strings.TrimSpace(name)))
	if e := ext(base); e != "" {
		base = strings.TrimSuffix(base, "."+e)
	}
	if base == "" {
		return Release{}, false
	}

	var r Release
	if m := reResolution.FindStringSubmatch(base); m != nil {
		r.Resolution = strings.ToLower(m[1])
	} else if reUHD.MatchString(base) {
		r.Resolution = "2160p"
	}

	if m := reBracketedGroup.FindStringSubmatch(base); m != nil {
		r.Group = foldToken(m[1])
	} else if m := reTrailingGroup.FindStringSubmatch(base); m != nil {
		r.Group = foldToken(m[1])
	}

	r.Key = ReleaseKey(name)
	if r.Resolution == "" || r.Group == "" {
		// The token path refuses it — but the key still identifies the file, which
		// is what a source that rewrites its release names leaves us.
		return Release{Key: r.Key}, false
	}
	return r, true
}

// Matches reports whether this release is the one an option offers.
//
// The option's own wording is normalized the same way, so a file's
// "x265-Silence" and an option's "Silence" agree while "TENEIGHTY" does not.
// Both halves must agree (FR-002).
func (r Release) Matches(resolution, encoder string) bool {
	if r.Resolution == "" || r.Group == "" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(resolution)) != r.Resolution {
		return false
	}
	g := foldToken(encoder)
	if g == "" {
		return false
	}
	// An option names its group either bare ("Silence") or with the codec still
	// attached ("x265-Silence"), so compare on the last token as well as the whole.
	if g == r.Group {
		return true
	}
	if i := strings.LastIndex(encoder, "-"); i >= 0 {
		return foldToken(encoder[i+1:]) == r.Group
	}
	return false
}

// foldToken reduces a group name to the form groups are compared in: lower case,
// with separators and punctuation dropped, so the same group written three ways
// still compares equal.
func foldToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ReleaseKey reduces a file name to what makes it THIS release.
//
// It exists because the tokens ReleaseOf reads can be overwritten by the source.
// One real source renames everything it serves with its own suffix, so files
// encoded by four different groups all report the same group, and labels every
// movie option with its own name — leaving nothing for a token comparison to tell
// apart. What it does not overwrite is that a given download link yields a given
// file, so comparing the files compares the releases.
//
// The reduction drops the extension and any directory, folds case and every run
// of punctuation away, and removes the episode number so a season downloaded in
// part still identifies itself. "" means the name says nothing at all.
func ReleaseKey(name string) string {
	base := strings.TrimSpace(path.Base(strings.TrimSpace(name)))
	if e := ext(base); e != "" {
		base = strings.TrimSuffix(base, "."+e)
	} else if strings.HasPrefix(base, ".") {
		// A name that is ALL extension (".mkv") is a dotfile with no stem — the
		// same rule ext() applies. There is no release here to identify.
		return ""
	}
	base = reEpisodeToken.ReplaceAllString(base, " s$1 ")
	base = reXEpisode.ReplaceAllString(base, " s$1 ")

	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(base) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte('-')
			}
			space = false
			b.WriteRune(r)
			continue
		}
		space = true
	}
	return b.String()
}

// RecordedRelease reads a release out of a name SynoDL itself recorded when the
// download was created — before anything renamed the library.
//
// It exists because the on-disk names cannot answer: a library renamed for a
// media server keeps no release information at all. The names we recorded do,
// but not in the shape ReleaseOf expects. Measured over 303 real ones, the last
// token is the SITE's brand (its tag on everything it serves) and the encoder
// sits immediately before it — "…x265.Silence-30nama.mkv".
//
// So the encoder is taken from that position rather than from the end. It is a
// heuristic, and it is safe to be one: the caller marks an option only when this
// release matches BOTH the option's resolution and its encoder, so a token picked
// out of the wrong place simply matches nothing.
func RecordedRelease(name string) (Release, bool) {
	base := strings.TrimSpace(path.Base(strings.TrimSpace(name)))
	if e := ext(base); e != "" {
		base = strings.TrimSuffix(base, "."+e)
	}
	var r Release
	if m := reResolution.FindStringSubmatch(base); m != nil {
		r.Resolution = strings.ToLower(m[1])
	} else if reUHD.MatchString(base) {
		r.Resolution = "2160p"
	}
	parts := reNameSeparators.Split(base, -1)
	// Drop empties so trailing separators do not shift the position.
	kept := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) >= 2 {
		r.Group = foldToken(kept[len(kept)-2])
	}
	r.Key = ReleaseKey(name)
	if r.Resolution == "" || r.Group == "" {
		return Release{Key: r.Key}, false
	}
	return r, true
}

var reNameSeparators = regexp.MustCompile(`[.\-_ ]+`)

package library

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// MediaKind says which parent folder a title belongs under. The caller maps the
// catalog's own type strings onto this, so this package stays independent of the
// source domain.
type MediaKind int

const (
	MediaMovie MediaKind = iota
	MediaTV              // series and anime both land under the TV parent
)

// trailingYear matches a release year or range at the END of a name, in every
// form the sources actually produce:
//
//	" 1986"   " (2014)"   " 2008 - 2013"   " 2008–2013"   " 2019 -"
//
// This is a deliberate port of YEAR_RE in src/services/title-year.ts — both are
// reading strings from the same sources, so they must agree. Change one and you
// must change the other.
var trailingYear = regexp.MustCompile(`\s*\(?\b((?:19|20)\d{2})\b(?:\s*[-–]\s*((?:19|20)\d{2})?)?\)?\s*$`)

// trailingGroup matches a trailing bracketed or parenthesised aside — "[1080p]",
// "(BluRay)", "(Director's Cut)". Stripped so presentation noise on disk does not
// hide a title we do have.
var trailingGroup = regexp.MustCompile(`\s*[\[(][^\[\]()]*[\])]\s*$`)

// leadingArticle is stripped so "The Batman" and "Batman" agree. The separator
// class is wider than whitespace because scene-style names join words with dots
// or underscores ("The.Matrix.1999"), and those must fold to the same key as the
// spaced form. Requiring a separator at all keeps "Theater" and "Anaconda"
// intact. Both sides of a comparison get this treatment, so it can only ever
// merge two names, never split them apart.
var leadingArticle = regexp.MustCompile(`^(?i:the|a|an)[\s._\-]+`)

// Key reduces a folder name or a catalog title to a comparison key, plus the
// release year when the name carries one.
//
// The key keeps only letters and digits, folded to lower case. Note the filter
// is unicode.IsLetter, NOT an ASCII range: the configured sources serve Persian
// titles, and an ASCII filter would reduce every one of them to the empty
// string — making them all collide with each other. That is a correctness bug,
// not a missing nicety.
//
// The returned year is the START year of a range ("2008" for "2008 - 2013"),
// because that is what identifies a title; the end year moves as a series runs.
func Key(name string) (key, year string) {
	s := strings.TrimSpace(name)
	if s == "" {
		return "", ""
	}

	// Peel trailing years and bracketed asides in whatever order they appear —
	// "Title (2019) [1080p]" needs two passes, and the bound stops a pathological
	// name from spinning.
	for i := 0; i < 4; i++ {
		if m := trailingYear.FindStringSubmatch(s); m != nil {
			rest := strings.TrimSpace(s[:len(s)-len(m[0])])
			// A name that is ONLY a year must not blank out — a folder called
			// "1917" is a real film, and an empty key would match every other
			// empty key. title-year.ts carries the same guard.
			if rest == "" {
				break
			}
			if year == "" {
				year = m[1]
			}
			s = rest
			continue
		}
		if m := trailingGroup.FindString(s); m != "" {
			rest := strings.TrimSpace(s[:len(s)-len(m)])
			if rest == "" {
				break
			}
			s = rest
			continue
		}
		break
	}

	s = leadingArticle.ReplaceAllString(s, "")
	return fold(s), year
}

// fold lower-cases and drops everything that is not a letter or a digit, so
// spacing, punctuation, and separators stop mattering.
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Parent is one configured destination folder and what it holds. Movies and TV
// may both be true: an operator is free to point both at the same folder.
type Parent struct {
	Path   string // share-relative, e.g. "movie"
	Movies bool
	TV     bool
}

// Entry is one title folder observed on the NAS.
type Entry struct {
	Name   string // exactly as DSM reported it
	Path   string // share-relative, e.g. "movie/Dune 2021"
	Parent string
	Year   string // start year parsed off Name; "" when it carries none
	Movies bool   // from the parent it was found under
	TV     bool
}

// Index is an immutable snapshot of the configured parents at one moment.
// A nil *Index is valid and matches nothing, so callers that failed to build one
// need no special case.
type Index struct {
	byKey   map[string][]Entry
	BuiltAt time.Time
	empty   bool
}

// Empty returns an index that matches nothing. It is what a failed read produces:
// "we could not look" is deliberately indistinguishable from "it is not there",
// because both mean the same thing to the UI — show no marker (FR-009).
func Empty(at time.Time) *Index {
	return &Index{byKey: map[string][]Entry{}, BuiltAt: at, empty: true}
}

// IsEmpty reports an index that knows nothing, whether because the read failed
// or because there was genuinely nothing there.
func (ix *Index) IsEmpty() bool {
	return ix == nil || ix.empty || len(ix.byKey) == 0
}

// Build indexes the folder names found under each parent. namesByParent is keyed
// by Parent.Path.
func Build(parents []Parent, namesByParent map[string][]string, at time.Time) *Index {
	ix := &Index{byKey: make(map[string][]Entry), BuiltAt: at}
	for _, p := range parents {
		for _, name := range namesByParent[p.Path] {
			k, y := Key(name)
			if k == "" {
				continue
			}
			ix.byKey[k] = append(ix.byKey[k], Entry{
				Name: name, Path: p.Path + "/" + name, Parent: p.Path,
				Year: y, Movies: p.Movies, TV: p.TV,
			})
		}
	}
	return ix
}

// Lookup reports whether a catalog title already exists under the parent that
// serves its media kind.
//
// The year rule is asymmetric on purpose. When BOTH sides carry a year they must
// agree, so "It 1990" never matches a folder holding the 2017 remake. When
// either side carries no year, the name match alone stands, so a plainly-named
// "Friends" folder still matches. Blocking on a MISSING year would hide most
// hand-made folders; allowing a MISMATCHED year would make a user skip a film
// they wanted. Only the second is unrecoverable.
func (ix *Index) Lookup(catalogTitle string, kind MediaKind) (Entry, bool) {
	if ix == nil || ix.empty {
		return Entry{}, false
	}
	k, wantYear := Key(catalogTitle)
	if k == "" {
		return Entry{}, false
	}
	for _, e := range ix.byKey[k] {
		if kind == MediaMovie && !e.Movies {
			continue
		}
		if kind == MediaTV && !e.TV {
			continue
		}
		if wantYear != "" && e.Year != "" && wantYear != e.Year {
			continue
		}
		return e, true
	}
	return Entry{}, false
}

// Folders returns every title folder in the index, as "parent/name".
//
// It exists for the background scan (spec 0011), which needs to know what there
// is to read and what has gone away, rather than to answer a lookup. Sorted, so
// a cycle is reproducible.
func (ix *Index) Folders() []string {
	if ix == nil || ix.empty {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, entries := range ix.byKey {
		for _, e := range entries {
			if e.Path == "" || seen[e.Path] {
				continue
			}
			seen[e.Path] = true
			out = append(out, e.Path)
		}
	}
	sort.Strings(out)
	return out
}

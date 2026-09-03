package library

import (
	"testing"
	"time"
)

// The trailing-year forms below are the ones src/services/title-year.ts already
// enumerates for the catalog's own titles; the Go side must agree with it, since
// both are reading strings produced by the same sources.
func TestKeyStripsTrailingYearForms(t *testing.T) {
	cases := []struct {
		name     string
		wantKey  string
		wantYear string
	}{
		{"Esther 1986", "esther", "1986"},
		{"Interstellar (2014)", "interstellar", "2014"},
		{"Breaking Bad 2008 - 2013", "breakingbad", "2008"},
		{"Breaking Bad 2008–2013", "breakingbad", "2008"}, // en dash
		{"Some Show 2019 -", "someshow", "2019"},          // still running
		{"Dune 2021", "dune", "2021"},
		{"No Year Here", "noyearhere", ""},
	}
	for _, c := range cases {
		gotKey, gotYear := Key(c.name)
		if gotKey != c.wantKey || gotYear != c.wantYear {
			t.Errorf("Key(%q) = (%q, %q), want (%q, %q)", c.name, gotKey, gotYear, c.wantKey, c.wantYear)
		}
	}
}

func TestKeyNormalisesPresentation(t *testing.T) {
	// Every one of these is the same title written differently on disk. They must
	// all reduce to the same key, or content that arrived outside SynoDL is
	// invisible — which is the whole point of the feature.
	same := []string{
		"The Matrix 1999",
		"the matrix 1999",
		"The  Matrix  (1999)",
		"The Matrix [1999]",
		"The.Matrix.1999",
		"The Matrix 1999 [1080p]",
		"The Matrix (1999) (BluRay)",
	}
	want, _ := Key(same[0])
	if want == "" {
		t.Fatal("baseline key is empty")
	}
	for _, s := range same[1:] {
		if got, _ := Key(s); got != want {
			t.Errorf("Key(%q) = %q, want %q", s, got, want)
		}
	}
}

// The configured sources serve Persian titles. An ASCII-range filter would
// reduce every one of them to the empty string, making them all collide — so
// this is a correctness requirement, not an internationalisation nicety.
func TestKeyKeepsNonLatinScripts(t *testing.T) {
	k1, y := Key("جدایی نادر از سیمین 2011")
	if k1 == "" {
		t.Fatal("Persian title reduced to an empty key")
	}
	if y != "2011" {
		t.Errorf("year = %q, want 2011", y)
	}
	k2, _ := Key("مارمولک 2004")
	if k2 == "" {
		t.Fatal("second Persian title reduced to an empty key")
	}
	if k1 == k2 {
		t.Error("two different Persian titles produced the same key")
	}
	if k, _ := Key("Bad Boys بد 1995"); k == "" {
		t.Error("mixed-script title reduced to an empty key")
	}
}

func TestKeyDoesNotBlankATitleThatIsOnlyAYear(t *testing.T) {
	// title-year.ts guards this case; so must we, or a folder named "1917"
	// becomes an empty key that matches everything else with an empty key.
	k, _ := Key("1917")
	if k == "" {
		t.Fatal(`Key("1917") reduced to an empty key`)
	}
	if other, _ := Key("2012"); k == other {
		t.Error("two year-only titles collapsed to the same key")
	}
}

func TestKeyStripsLeadingArticle(t *testing.T) {
	a, _ := Key("The Batman 2022")
	b, _ := Key("Batman 2022")
	if a != b {
		t.Errorf("leading article not stripped: %q vs %q", a, b)
	}
}

// ---- Lookup -------------------------------------------------------------

func idx(movies, tv []string) *Index {
	return Build(
		[]Parent{{Path: "movie", Movies: true}, {Path: "tv-show", TV: true}},
		map[string][]string{"movie": movies, "tv-show": tv},
		time.Now(),
	)
}

// The rule that stops the one failure this feature cannot afford.
func TestLookupRequiresYearsToAgreeWhenBothCarryOne(t *testing.T) {
	ix := idx([]string{"It 2017"}, nil)

	if _, ok := ix.Lookup("It 2017", MediaMovie); !ok {
		t.Error("same title and year did not match")
	}
	if e, ok := ix.Lookup("It 1990", MediaMovie); ok {
		t.Errorf("It 1990 wrongly matched %q — a false positive makes a user skip a title they wanted", e.Name)
	}
}

func TestLookupMatchesWhenEitherSideHasNoYear(t *testing.T) {
	ix := idx(nil, []string{"Friends"}) // folder carries no year
	if _, ok := ix.Lookup("Friends 1994 - 2004", MediaTV); !ok {
		t.Error("a year-less folder should still match a catalog title that carries one")
	}
	ix2 := idx(nil, []string{"The Wire 2002 - 2008"})
	if _, ok := ix2.Lookup("The Wire", MediaTV); !ok {
		t.Error("a year-less catalog title should still match a folder that carries one")
	}
}

func TestLookupSeparatesMoviesFromTV(t *testing.T) {
	ix := idx([]string{"Dune 2021"}, []string{"Friends 1994"})
	if _, ok := ix.Lookup("Dune 2021", MediaTV); ok {
		t.Error("a movie folder answered a TV lookup")
	}
	if _, ok := ix.Lookup("Friends 1994", MediaMovie); ok {
		t.Error("a TV folder answered a movie lookup")
	}
}

func TestLookupHandlesSharedParent(t *testing.T) {
	// An operator may point both parents at one folder; a lookup of either kind
	// must then find it.
	ix := Build(
		[]Parent{{Path: "media", Movies: true, TV: true}},
		map[string][]string{"media": {"Dune 2021"}},
		time.Now(),
	)
	if _, ok := ix.Lookup("Dune 2021", MediaMovie); !ok {
		t.Error("shared parent did not answer a movie lookup")
	}
	if _, ok := ix.Lookup("Dune 2021", MediaTV); !ok {
		t.Error("shared parent did not answer a TV lookup")
	}
}

func TestLookupReturnsThePathForAMatch(t *testing.T) {
	ix := idx([]string{"Dune 2021"}, nil)
	e, ok := ix.Lookup("Dune 2021", MediaMovie)
	if !ok {
		t.Fatal("no match")
	}
	if e.Path != "movie/Dune 2021" {
		t.Errorf("Path = %q, want movie/Dune 2021", e.Path)
	}
}

// Two distinct catalog titles can sanitize to one folder name. This already
// happens on send (both share a destination), so the index reports the system's
// real behaviour rather than inventing a new inconsistency.
func TestLookupToleratesCollidingNames(t *testing.T) {
	ix := idx([]string{"Up 2009", "Up! 2009"}, nil)
	if _, ok := ix.Lookup("Up 2009", MediaMovie); !ok {
		t.Error("colliding names should still match, not error")
	}
}

// An index built from a failed read must match nothing — "we could not look" is
// reported as "not present", never as an error the user sees (FR-009).
func TestEmptyIndexMatchesNothing(t *testing.T) {
	ix := Empty(time.Now())
	if !ix.IsEmpty() {
		t.Error("Empty() index does not report itself as empty")
	}
	if _, ok := ix.Lookup("Dune 2021", MediaMovie); ok {
		t.Error("an empty index matched a title")
	}
}

func TestLookupIgnoresBlankTitles(t *testing.T) {
	ix := idx([]string{"Dune 2021"}, nil)
	for _, s := range []string{"", "   ", "\t"} {
		if _, ok := ix.Lookup(s, MediaMovie); ok {
			t.Errorf("blank title %q matched", s)
		}
	}
}

func TestNilIndexIsSafe(t *testing.T) {
	// The API layer degrades to a nil index on any failure; it must not panic.
	var ix *Index
	if _, ok := ix.Lookup("Dune 2021", MediaMovie); ok {
		t.Error("nil index matched")
	}
	if !ix.IsEmpty() {
		t.Error("nil index should report empty")
	}
}

// The library is being tidied into the Plex/Jellyfin convention — "Dune (2021)",
// "Friends (1994)" — which is NOT the shape SynoDL itself writes. The ownership
// markers must keep working across that rename, or tidying the library silently
// switches the feature off.
func TestLookupSurvivesTheTidyToPlexNaming(t *testing.T) {
	// Folders as they look AFTER the tidy; catalog titles as the sources send them.
	ix := Build(
		[]Parent{{Path: "movie", Movies: true}, {Path: "tv-show", TV: true}},
		map[string][]string{
			"movie":   {"Dune (2021)", "Blade Runner 2049 (2017)", "WALL-E (2008)"},
			"tv-show": {"Friends (1994)", "The Bear (2022)"},
		},
		time.Now(),
	)
	for _, c := range []struct {
		catalogTitle string
		kind         MediaKind
	}{
		{"Dune 2021", MediaMovie},
		{"Blade Runner 2049 2017", MediaMovie},
		{"WALL-E 2008", MediaMovie},
		// A series' catalog title carries the whole run; the tidied folder keeps
		// only the first air year, so the two must still agree.
		{"Friends 1994 - 2004", MediaTV},
		{"The Bear 2022 -", MediaTV},
	} {
		if _, ok := ix.Lookup(c.catalogTitle, c.kind); !ok {
			t.Errorf("%q no longer matches its tidied folder — the badge would go dark", c.catalogTitle)
		}
	}
	// And the year rule still holds after tidying.
	if _, ok := ix.Lookup("Dune 1984", MediaMovie); ok {
		t.Error("the 1984 Dune matched the 2021 folder")
	}
}

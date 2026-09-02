package source

import "testing"

func opts(pairs ...[2]string) []FacetOption {
	out := make([]FacetOption, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, FacetOption{Value: p[0], Slug: p[0], Name: p[1]})
	}
	return out
}

// FR-014: combined mode must offer only what every source understands, or a
// filter silently leaves one source's results unfiltered.
func TestIntersectParametersKeepsOnlySharedOptions(t *testing.T) {
	a := SearchParameters{Genres: opts([2]string{"action", "Action"}, [2]string{"drama", "Drama"}, [2]string{"anime", "Anime"})}
	b := SearchParameters{Genres: opts([2]string{"drama", "Drama"}, [2]string{"comedy", "Comedy"}, [2]string{"action", "Action"})}
	got := IntersectParameters([]SearchParameters{a, b})
	if len(got.Genres) != 2 {
		t.Fatalf("genres = %+v, want the two shared ones", got.Genres)
	}
	seen := map[string]bool{}
	for _, g := range got.Genres {
		seen[g.Slug] = true
	}
	if !seen["action"] || !seen["drama"] || seen["anime"] || seen["comedy"] {
		t.Fatalf("wrong intersection: %+v", got.Genres)
	}
}

// A facet kind one source lacks entirely must drop out — otherwise the filter
// would apply to some results and not others.
func TestIntersectParametersDropsUnsupportedFacetKind(t *testing.T) {
	a := SearchParameters{Qualities: opts([2]string{"1080p", "1080p"}), Genres: opts([2]string{"action", "Action"})}
	b := SearchParameters{Genres: opts([2]string{"action", "Action"})} // no qualities at all
	got := IntersectParameters([]SearchParameters{a, b})
	if len(got.Qualities) != 0 {
		t.Fatalf("qualities survived though one source has none: %+v", got.Qualities)
	}
	if len(got.Genres) != 1 {
		t.Fatalf("genres = %+v", got.Genres)
	}
}

// Sources label the same thing differently — one in English, one not. Matching
// is by slug where there is one, so equivalent options collapse to a single
// entry rather than appearing twice.
func TestIntersectParametersMatchesBySlugAndEmitsOnce(t *testing.T) {
	a := SearchParameters{Genres: []FacetOption{{Value: "12", Slug: "action", Name: "Action"}}}
	b := SearchParameters{Genres: []FacetOption{{Value: "99", Slug: "action", Name: "اکشن"}}}
	got := IntersectParameters([]SearchParameters{a, b})
	if len(got.Genres) != 1 {
		t.Fatalf("equivalent options should appear once, got %+v", got.Genres)
	}
}

// Without slugs, fall back to a normalized label so differently-cased or spaced
// labels still match.
func TestIntersectParametersFallsBackToNormalizedLabel(t *testing.T) {
	a := SearchParameters{Encoders: []FacetOption{{Value: "1", Name: "YIFY"}}}
	b := SearchParameters{Encoders: []FacetOption{{Value: "2", Name: "  yify  "}}}
	got := IntersectParameters([]SearchParameters{a, b})
	if len(got.Encoders) != 1 {
		t.Fatalf("labels differing only by case/space should match: %+v", got.Encoders)
	}
}

// The usable year range is the one every source can serve.
func TestIntersectParametersNarrowsYearBounds(t *testing.T) {
	a := SearchParameters{MinYear: 1900, MaxYear: 2026}
	b := SearchParameters{MinYear: 1970, MaxYear: 2020}
	got := IntersectParameters([]SearchParameters{a, b})
	if got.MinYear != 1970 || got.MaxYear != 2020 {
		t.Fatalf("years = %d–%d, want 1970–2020", got.MinYear, got.MaxYear)
	}
}

// One source is passed through untouched — there is nothing to intersect.
func TestIntersectParametersSingleSourceUnchanged(t *testing.T) {
	a := SearchParameters{Genres: opts([2]string{"action", "Action"}), Qualities: opts([2]string{"4k", "4K"})}
	got := IntersectParameters([]SearchParameters{a})
	if len(got.Genres) != 1 || len(got.Qualities) != 1 {
		t.Fatalf("single source should pass through: %+v", got)
	}
}

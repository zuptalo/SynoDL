package api

import (
	"context"
	"testing"

	"synodl/server/internal/source"
)

// capsProvider is a Provider that only has to answer "what can you filter by?".
type capsProvider struct {
	params source.SearchParameters
	err    error
	asked  *int
}

func (capsProvider) Kind() string                         { return "caps" }
func (capsProvider) DisplayName() string                  { return "Caps" }
func (capsProvider) SessionFields() []source.SessionField { return nil }
func (capsProvider) Hosts() source.Config                 { return source.Config{} }
func (capsProvider) VerifySession(context.Context, *source.Client, source.Config, source.Session) error {
	return nil
}
func (p capsProvider) Parameters(context.Context, *source.Client, source.Config, source.Session) (source.SearchParameters, error) {
	if p.asked != nil {
		*p.asked++
	}
	return p.params, p.err
}
func (capsProvider) Search(context.Context, *source.Client, source.Config, source.Session, source.SearchQuery) (source.SearchResult, error) {
	return source.SearchResult{}, nil
}
func (capsProvider) Title(context.Context, *source.Client, source.Config, source.Session, string) (source.TitleDetail, error) {
	return source.TitleDetail{}, nil
}
func (capsProvider) ResolveDownload(context.Context, *source.Client, source.Config, source.Session, string, string) ([]string, string, error) {
	return nil, "", nil
}

// The two real vocabularies, in miniature: numeric codes on one source, the
// site's own words on the other, joined by an English slug.
func codeSource(asked *int) source.SourceRef {
	return source.SourceRef{ID: 1, Name: "codes", Driver: capsProvider{asked: asked, params: source.SearchParameters{
		Genres: []source.FacetOption{
			{Value: "3359", Name: "کمدی", Slug: "comedy"},
			{Value: "3362", Name: "درام", Slug: "drama"},
			{Value: "3377", Name: "ابرقهرمانی", Slug: "superhero"},
		},
		Scores: []source.FacetOption{{Value: "8", Slug: "score-8"}, {Value: "8.5", Slug: "score-8.5"}},
	}}}
}

func wordSource(asked *int) source.SourceRef {
	return source.SourceRef{ID: 2, Name: "words", Driver: capsProvider{asked: asked, params: source.SearchParameters{
		Genres: []source.FacetOption{
			{Value: "کمدی", Name: "کمدی", Slug: "comedy"},
			{Value: "درام", Name: "درام", Slug: "drama"},
		},
		Scores: []source.FacetOption{{Value: "8", Slug: "score-8"}},
	}}}
}

func TestPerSourceFiltersTranslatesIntoEachVocabulary(t *testing.T) {
	d := Deps{caps: &capsCache{}}
	refs := []source.SourceRef{codeSource(nil), wordSource(nil)}

	got := d.perSourceFilters(context.Background(), refs, source.SearchFilters{Genre: []string{"3359"}, Score: "8"})

	if got[1] == nil || got[2] == nil {
		t.Fatalf("both sources can serve comedy: %+v", got)
	}
	if got[1].Genre[0] != "3359" {
		t.Fatalf("code source got %q", got[1].Genre[0])
	}
	if got[2].Genre[0] != "کمدی" {
		t.Fatalf("word source got %q, want its own wording", got[2].Genre[0])
	}
	// A value both spell the same still round-trips untouched.
	if got[1].Score != "8" || got[2].Score != "8" {
		t.Fatalf("scores = %q / %q", got[1].Score, got[2].Score)
	}
}

// FR-007: a source with no word for the chosen genre is ruled out rather than
// queried without it.
func TestPerSourceFiltersRulesOutASourceThatCannotExpressTheChoice(t *testing.T) {
	d := Deps{caps: &capsCache{}}
	refs := []source.SourceRef{codeSource(nil), wordSource(nil)}

	got := d.perSourceFilters(context.Background(), refs, source.SearchFilters{Genre: []string{"3377"}})

	if got[1] == nil || got[1].Genre[0] != "3377" {
		t.Fatalf("the source that has superhero must keep it: %+v", got[1])
	}
	if got[2] != nil {
		t.Fatalf("the source without superhero must be ruled out, got %+v", got[2])
	}
}

// A value no source recognises is passed through rather than guessed at: it may
// be a stored choice from a facet list that has since changed.
func TestPerSourceFiltersPassesThroughUnknownValues(t *testing.T) {
	d := Deps{caps: &capsCache{}}
	refs := []source.SourceRef{codeSource(nil), wordSource(nil)}

	got := d.perSourceFilters(context.Background(), refs, source.SearchFilters{Genre: []string{"9999"}})

	for id, f := range got {
		if f == nil || f.Genre[0] != "9999" {
			t.Fatalf("source %d: %+v", id, f)
		}
	}
}

// FR-011: one source failing to report its abilities must not rule it out, and
// must not disturb the others.
func TestPerSourceFiltersKeepsASourceWhoseAbilitiesAreUnknown(t *testing.T) {
	d := Deps{caps: &capsCache{}}
	broken := source.SourceRef{ID: 3, Name: "broken", Driver: capsProvider{err: context.DeadlineExceeded}}
	refs := []source.SourceRef{codeSource(nil), broken}

	got := d.perSourceFilters(context.Background(), refs, source.SearchFilters{Genre: []string{"3359"}})

	if got[3] == nil {
		t.Fatal("unknown abilities is not the same as no equivalent; the source must still be tried")
	}
	if got[1] == nil || got[1].Genre[0] != "3359" {
		t.Fatalf("the healthy source is unaffected: %+v", got[1])
	}
}

// A single source needs no translation at all — and must not be made to pay for
// a capability fetch to discover that.
func TestPerSourceFiltersSkipsTheSingleSourceCase(t *testing.T) {
	asked := 0
	d := Deps{caps: &capsCache{}}
	got := d.perSourceFilters(context.Background(), []source.SourceRef{codeSource(&asked)}, source.SearchFilters{Genre: []string{"3359"}})
	if got != nil {
		t.Fatalf("expected no overrides, got %+v", got)
	}
	if asked != 0 {
		t.Fatalf("asked the source %d times; browsing one source must not fetch its abilities", asked)
	}
}

// Browsing must not re-ask every source on every request.
func TestSourceCapsAreCachedAndInvalidatable(t *testing.T) {
	asked := 0
	d := Deps{caps: &capsCache{}}
	ref := codeSource(&asked)
	for i := 0; i < 3; i++ {
		if _, ok := d.sourceCaps(context.Background(), ref); !ok {
			t.Fatal("capabilities should be readable")
		}
	}
	if asked != 1 {
		t.Fatalf("asked %d times, want 1", asked)
	}
	d.invalidateCaps(ref.ID)
	if _, ok := d.sourceCaps(context.Background(), ref); !ok {
		t.Fatal("capabilities should be re-readable")
	}
	if asked != 2 {
		t.Fatalf("asked %d times after invalidation, want 2", asked)
	}
}

// A failure must not be cached: the next request has to try again, or one blip
// blanks a source's abilities for the whole TTL.
func TestSourceCapsDoNotCacheFailures(t *testing.T) {
	asked := 0
	d := Deps{caps: &capsCache{}}
	ref := source.SourceRef{ID: 9, Driver: capsProvider{asked: &asked, err: context.DeadlineExceeded}}
	for i := 0; i < 3; i++ {
		if _, ok := d.sourceCaps(context.Background(), ref); ok {
			t.Fatal("expected failure")
		}
	}
	if asked != 3 {
		t.Fatalf("asked %d times, want 3 — failures must not be cached", asked)
	}
}

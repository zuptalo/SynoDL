package source

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeProvider is a Provider that answers from canned data, so the merge layer
// is tested with no HTTP and no real driver.
type fakeProvider struct {
	kind  string
	items []CatalogTitle
	pages int
	err   error
	delay time.Duration
	// search, when set, answers instead of the canned data — for tests that care
	// about the QUERY each source was handed rather than what came back.
	search func(SearchQuery) (SearchResult, error)
}

func (f fakeProvider) Kind() string                  { return f.kind }
func (f fakeProvider) DisplayName() string           { return f.kind }
func (f fakeProvider) SessionFields() []SessionField { return nil }
func (f fakeProvider) Hosts() Config                 { return Config{} }
func (f fakeProvider) VerifySession(context.Context, *Client, Config, Session) error {
	return nil
}
func (f fakeProvider) Parameters(context.Context, *Client, Config, Session) (SearchParameters, error) {
	return SearchParameters{}, nil
}
func (f fakeProvider) Title(context.Context, *Client, Config, Session, string) (TitleDetail, error) {
	return TitleDetail{}, nil
}
func (f fakeProvider) ResolveDownload(context.Context, *Client, Config, Session, string, string) ([]string, string, error) {
	return nil, "", nil
}
func (f fakeProvider) Search(ctx context.Context, _ *Client, _ Config, _ Session, q SearchQuery) (SearchResult, error) {
	if f.search != nil {
		return f.search(q)
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return SearchResult{}, ctx.Err()
		}
	}
	if f.err != nil {
		return SearchResult{}, f.err
	}
	return SearchResult{Page: 1, Pages: f.pages, Items: f.items}, nil
}

func titles(prefix string, n int) []CatalogTitle {
	out := make([]CatalogTitle, n)
	for i := range out {
		out[i] = CatalogTitle{ID: fmt.Sprintf("%s%d", prefix, i), Title: fmt.Sprintf("%s %d", prefix, i)}
	}
	return out
}

func ref(id int64, name string, p Provider) SourceRef {
	return SourceRef{ID: id, Name: name, Driver: p}
}

func ids(items []CatalogTitle) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestInterleaveRoundRobin(t *testing.T) {
	got := Interleave([][]CatalogTitle{titles("a", 3), titles("b", 3)})
	want := []string{"a0", "b0", "a1", "b1", "a2", "b2"}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Fatalf("interleave = %v, want %v", ids(got), want)
	}
}

// Sources paginate differently (the real ones return different page sizes), so a
// short source must not truncate a long one — nor stop contributing early.
func TestInterleaveUnevenPageSizes(t *testing.T) {
	got := Interleave([][]CatalogTitle{titles("a", 4), titles("b", 1)})
	want := []string{"a0", "b0", "a1", "a2", "a3"}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Fatalf("interleave = %v, want %v", ids(got), want)
	}
	if len(got) != 5 {
		t.Fatalf("lost items: got %d, want 5", len(got))
	}
}

func TestInterleaveExhaustedSourceContributesNothing(t *testing.T) {
	got := Interleave([][]CatalogTitle{titles("a", 2), nil})
	if fmt.Sprint(ids(got)) != fmt.Sprint([]string{"a0", "a1"}) {
		t.Fatalf("interleave = %v", ids(got))
	}
	if Interleave(nil) != nil {
		t.Fatal("empty input should yield nil")
	}
}

// FR-005a: a title carried by two sources yields TWO entries. The merge must not
// de-duplicate — the entries offer different releases and different downloads,
// and collapsing them would hide one. This test exists because the behavior is
// satisfied by an ABSENCE of code, which is exactly what a later well-meaning
// "fix" would add.
func TestSearchAllDoesNotDeduplicateAcrossSources(t *testing.T) {
	ResetBreakers()
	same := []CatalogTitle{{ID: "the-matrix-1999", IMDbID: "tt0133093", Title: "The Matrix"}}
	refs := []SourceRef{
		ref(1, "Alpha", fakeProvider{kind: "a", items: same, pages: 1}),
		ref(2, "Beta", fakeProvider{kind: "b", items: same, pages: 1}),
	}
	res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
	if len(res.Items) != 2 {
		t.Fatalf("got %d items, want 2 (one per source, never merged)", len(res.Items))
	}
	if res.Items[0].SourceID == res.Items[1].SourceID {
		t.Fatal("both entries attributed to the same source")
	}
	if res.Items[0].ID == res.Items[1].ID {
		t.Fatalf("entries must be separately addressable, both were %q", res.Items[0].ID)
	}
	for _, it := range res.Items {
		if it.SourceName == "" {
			t.Fatal("source label missing; combined mode would look like a duplicate")
		}
	}
}

// FR-010: with one source there is nothing to interleave, and the source's own
// ordering must survive untouched.
func TestSearchAllSingleSourcePreservesOrder(t *testing.T) {
	ResetBreakers()
	in := titles("a", 5)
	res := SearchAll(context.Background(), nil, []SourceRef{
		ref(1, "Alpha", fakeProvider{kind: "a", items: in, pages: 3}),
	}, SearchQuery{Page: 1})
	want := []string{"1:a0", "1:a1", "1:a2", "1:a3", "1:a4"}
	if fmt.Sprint(ids(res.Items)) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", ids(res.Items), want)
	}
}

// FR-012: a failing source must never blank a view another source could fill.
func TestSearchAllDegradesInsteadOfFailing(t *testing.T) {
	ResetBreakers()
	refs := []SourceRef{
		ref(1, "Healthy", fakeProvider{kind: "a", items: titles("a", 2), pages: 4}),
		ref(2, "Broken", fakeProvider{kind: "b", err: &ErrNeedsRefresh{Layer: LayerToken}}),
	}
	res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
	if len(res.Items) != 2 {
		t.Fatalf("healthy source lost its items: got %d", len(res.Items))
	}
	if len(res.Degraded) != 1 || res.Degraded[0].Name != "Broken" {
		t.Fatalf("degraded = %+v, want the broken source named", res.Degraded)
	}
	if res.Degraded[0].Reason != ReasonNeedsRefresh {
		t.Fatalf("reason = %q, want %q", res.Degraded[0].Reason, ReasonNeedsRefresh)
	}
	if res.Pages != 4 {
		t.Fatalf("pages = %d, want the healthy source's 4", res.Pages)
	}
}

// FR-030: a slow source is bounded by the per-source timeout and reported, not
// waited on. Uses a delay well past the timeout with a shortened deadline via a
// parent context, so the test stays fast.
func TestSearchAllTimesOutSlowSource(t *testing.T) {
	ResetBreakers()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	refs := []SourceRef{
		ref(1, "Fast", fakeProvider{kind: "a", items: titles("a", 2), pages: 1}),
		ref(2, "Slow", fakeProvider{kind: "b", delay: 5 * time.Second}),
	}
	start := time.Now()
	res := SearchAll(ctx, nil, refs, SearchQuery{Page: 1})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("combined query waited %v on a slow source", elapsed)
	}
	if len(res.Items) != 2 {
		t.Fatalf("fast source lost its items: got %d", len(res.Items))
	}
	if len(res.Degraded) != 1 || res.Degraded[0].SourceID != 2 {
		t.Fatalf("slow source not reported as degraded: %+v", res.Degraded)
	}
}

// Every source failing yields nothing plus a full degraded list, so the caller
// can report the failure once rather than once per source.
func TestSearchAllEverySourceFailing(t *testing.T) {
	ResetBreakers()
	refs := []SourceRef{
		ref(1, "A", fakeProvider{kind: "a", err: errors.New("boom")}),
		ref(2, "B", fakeProvider{kind: "b", err: errors.New("boom")}),
	}
	res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
	if len(res.Items) != 0 || len(res.Degraded) != 2 {
		t.Fatalf("items=%d degraded=%d, want 0 and 2", len(res.Items), len(res.Degraded))
	}
}

// FR-031: a source that keeps failing enters a cooling-off window and stops
// being called, so a down source neither slows every search nor hammers an
// upstream that is already struggling. It is still REPORTED while cooling off —
// the user is told, not quietly shown less.
func TestSearchAllCoolsOffRepeatedlyFailingSource(t *testing.T) {
	ResetBreakers()
	calls := 0
	counting := countingProvider{onSearch: func() { calls++ }}
	refs := []SourceRef{ref(9, "Flaky", counting)}

	for i := 0; i < coolOffThreshold+3; i++ {
		res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
		if len(res.Degraded) != 1 {
			t.Fatalf("iteration %d: expected the source reported as degraded", i)
		}
	}
	if calls > coolOffThreshold {
		t.Fatalf("kept calling a failing source: %d calls, want at most %d", calls, coolOffThreshold)
	}
	// A successful call clears the breaker so recovery is immediate.
	ResetBreakers()
	if breakerOpen(9, time.Now()) {
		t.Fatal("breaker still open after reset")
	}
}

type countingProvider struct {
	fakeProvider
	onSearch func()
}

func (c countingProvider) Kind() string        { return "counting" }
func (c countingProvider) DisplayName() string { return "counting" }
func (c countingProvider) Search(context.Context, *Client, Config, Session, SearchQuery) (SearchResult, error) {
	c.onSearch()
	return SearchResult{}, errors.New("always fails")
}

// FR-032: one client query causes at most one page fetch per source — never a
// per-item or per-title fan-out.
func TestSearchAllOneFetchPerSource(t *testing.T) {
	ResetBreakers()
	var a, b int
	refs := []SourceRef{
		ref(1, "A", tallyProvider{n: &a, items: titles("a", 20)}),
		ref(2, "B", tallyProvider{n: &b, items: titles("b", 20)}),
	}
	SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
	if a != 1 || b != 1 {
		t.Fatalf("fetches per source = %d/%d, want 1/1", a, b)
	}
}

type tallyProvider struct {
	fakeProvider
	n     *int
	items []CatalogTitle
}

func (t tallyProvider) Kind() string        { return "tally" }
func (t tallyProvider) DisplayName() string { return "tally" }
func (t tallyProvider) Search(context.Context, *Client, Config, Session, SearchQuery) (SearchResult, error) {
	*t.n++
	return SearchResult{Page: 1, Pages: 1, Items: t.items}, nil
}

// Spec 1024: a slug and an equivalent label must be judged the same option.
// They used to live in separate key spaces ("slug:comedy" could never equal
// "name:Comedy"), so two sources joined only when BOTH supplied a slug — and a
// source that supplies only labels silently failed to intersect with anything.
func TestFacetKeyJoinsSlugsAndLabels(t *testing.T) {
	same := [][2]FacetOption{
		{{Slug: "comedy"}, {Slug: "Comedy"}},
		{{Slug: "comedy"}, {Name: "Comedy"}},
		{{Name: "Sci-Fi"}, {Slug: "sci-fi"}},
		{{Name: "Sci Fi"}, {Slug: "sci-fi"}},
		{{Name: "  Film   Noir "}, {Slug: "film-noir"}},
		// The value is provider-internal and must never enter the key.
		{{Value: "3359", Slug: "comedy"}, {Value: "کمدی", Slug: "comedy"}},
	}
	for _, p := range same {
		if facetKey(p[0]) != facetKey(p[1]) {
			t.Fatalf("%+v and %+v should key alike: %q vs %q", p[0], p[1], facetKey(p[0]), facetKey(p[1]))
		}
	}
	differ := [][2]FacetOption{
		{{Slug: "comedy"}, {Slug: "crime"}},
		{{Name: "Comedy"}, {Name: "کمدی"}}, // different languages do not join
		{{Slug: "comedy"}, {}},
	}
	for _, p := range differ {
		if facetKey(p[0]) == facetKey(p[1]) {
			t.Fatalf("%+v and %+v must not key alike (%q)", p[0], p[1], facetKey(p[0]))
		}
	}
}

// A sort only one source can honour must drop from the combined set, exactly as
// a genre does — otherwise picking it would order one source's results and leave
// the other's in whatever order it pleased.
func TestIntersectParametersFoldsInSorts(t *testing.T) {
	a := SearchParameters{
		Types: []FacetOption{{Slug: "movie", Name: "Movie"}},
		Sorts: []FacetOption{{Slug: "imdb", Name: "IMDb rating"}, {Slug: "year", Name: "Release year"}},
	}
	b := SearchParameters{
		Types: []FacetOption{{Slug: "movie", Name: "Movie"}},
		Sorts: []FacetOption{{Slug: "imdb", Name: "IMDb rating"}, {Slug: "modified", Name: "Recently updated"}},
	}
	got := IntersectParameters([]SearchParameters{a, b})
	if len(got.Sorts) != 1 || got.Sorts[0].Slug != "imdb" {
		t.Fatalf("sorts = %+v, want only imdb", got.Sorts)
	}
	// A single source keeps everything it offers.
	if solo := IntersectParameters([]SearchParameters{b}); len(solo.Sorts) != 2 {
		t.Fatalf("single source sorts = %+v, want both", solo.Sorts)
	}
}

// Spec 1024 FR-006/FR-007: two sources spell the same genre differently, so the
// API layer hands each one its OWN value. A source with no equivalent for the
// chosen value must be skipped and reported — never queried unfiltered, which
// would present unfiltered results as filtered.
func TestSearchAllUsesPerSourceFilters(t *testing.T) {
	seen := map[int64]string{}
	var mu sync.Mutex
	mk := func(id int64) SourceRef {
		return SourceRef{ID: id, Name: fmt.Sprintf("src%d", id), Driver: &fakeProvider{
			search: func(q SearchQuery) (SearchResult, error) {
				mu.Lock()
				seen[id] = firstOr(q.Filters.Genre, "")
				mu.Unlock()
				return SearchResult{Items: []CatalogTitle{{ID: "x", Title: "T"}}}, nil
			},
		}}
	}
	refs := []SourceRef{mk(1), mk(2), mk(3)}
	res := SearchAll(context.Background(), NewClient(), refs, SearchQuery{
		Filters: SearchFilters{Genre: []string{"comedy"}},
		PerSource: map[int64]*SearchFilters{
			1: {Genre: []string{"3359"}}, // this source's numeric code
			2: {Genre: []string{"کمدی"}}, // this source's own wording
			3: nil,                       // no equivalent at all
		},
	})
	if seen[1] != "3359" || seen[2] != "کمدی" {
		t.Fatalf("each source must get its own value; got %+v", seen)
	}
	if _, queried := seen[3]; queried {
		t.Fatal("a source with no equivalent must not be queried at all")
	}
	var reported bool
	for _, d := range res.Degraded {
		if d.SourceID == 3 && d.Reason == ReasonFilterUnsupported {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("source 3 must be reported, got %+v", res.Degraded)
	}
	// The two that could answer still did.
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(res.Items))
	}
}

// A ref absent from the override map falls back to the shared filters, so the
// single-source path needs no special casing.
func TestSearchAllWithoutOverridesUsesSharedFilters(t *testing.T) {
	var got string
	refs := []SourceRef{{ID: 1, Name: "only", Driver: &fakeProvider{
		search: func(q SearchQuery) (SearchResult, error) {
			got = firstOr(q.Filters.Genre, "")
			return SearchResult{}, nil
		},
	}}}
	SearchAll(context.Background(), NewClient(), refs, SearchQuery{Filters: SearchFilters{Genre: []string{"drama"}}})
	if got != "drama" {
		t.Fatalf("genre = %q, want drama", got)
	}
}

func firstOr(v []string, def string) string {
	if len(v) > 0 {
		return v[0]
	}
	return def
}

// Spec 2032 FR-001/FR-002, SC-001. THE REGRESSION THIS EXISTS FOR.
//
// In production a source whose session had expired reported "unreachable" for
// hours, which the client renders as "isn't responding" — so the operator went
// looking at a website that was serving its whole catalogue the entire time.
//
// classify() gets the reason right on the FIRST attempt. The breaker then threw
// it away: it recorded a count and a deadline and nothing else, so once it
// opened, every later request short-circuited to a flat ReasonUnreachable.
func TestBreakerKeepsTheReasonThatOpenedIt(t *testing.T) {
	ResetBreakers()
	refs := []SourceRef{
		ref(1, "Stale", fakeProvider{kind: "s", err: &ErrNeedsRefresh{Layer: LayerToken}}),
	}

	// Fail it past the threshold, so the breaker is open and calls stop.
	for i := 0; i < coolOffThreshold+2; i++ {
		res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
		if len(res.Degraded) != 1 {
			t.Fatalf("attempt %d: degraded = %+v, want one entry", i, res.Degraded)
		}
		if got := res.Degraded[0].Reason; got != ReasonNeedsRefresh {
			t.Fatalf("attempt %d: reason = %q, want %q — the breaker flattened it",
				i, got, ReasonNeedsRefresh)
		}
	}
}

// FR-004: two sources failing differently must each keep their own reason.
func TestBreakerReasonsAreNotSharedBetweenSources(t *testing.T) {
	ResetBreakers()
	refs := []SourceRef{
		ref(1, "Stale", fakeProvider{kind: "s", err: &ErrNeedsRefresh{Layer: LayerToken}}),
		ref(2, "Gone", fakeProvider{kind: "g", err: errors.New("dial tcp: no route to host")}),
	}
	for i := 0; i < coolOffThreshold+2; i++ {
		res := SearchAll(context.Background(), nil, refs, SearchQuery{Page: 1})
		got := map[string]string{}
		for _, d := range res.Degraded {
			got[d.Name] = d.Reason
		}
		if got["Stale"] != ReasonNeedsRefresh || got["Gone"] != ReasonUnreachable {
			t.Fatalf("attempt %d: reasons = %+v, want Stale=%s Gone=%s",
				i, got, ReasonNeedsRefresh, ReasonUnreachable)
		}
	}
}

// FR-003: a source that recovers forgets the reason with everything else.
func TestBreakerSuccessClearsTheRememberedReason(t *testing.T) {
	ResetBreakers()
	broken := []SourceRef{ref(1, "Flaky", fakeProvider{kind: "f", err: &ErrNeedsRefresh{Layer: LayerToken}})}
	for i := 0; i < coolOffThreshold+1; i++ {
		SearchAll(context.Background(), nil, broken, SearchQuery{Page: 1})
	}
	healthy := []SourceRef{ref(1, "Flaky", fakeProvider{kind: "f", items: titles("f", 1), pages: 1})}
	ResetBreakers() // an admin re-saving the source, which is what clears it in practice
	res := SearchAll(context.Background(), nil, healthy, SearchQuery{Page: 1})
	if len(res.Degraded) != 0 {
		t.Fatalf("degraded = %+v, want none once it works again", res.Degraded)
	}
	// And failing again starts from a clean reason rather than the stale one.
	again := []SourceRef{ref(1, "Flaky", fakeProvider{kind: "f", err: errors.New("dial tcp: refused")})}
	res = SearchAll(context.Background(), nil, again, SearchQuery{Page: 1})
	if res.Degraded[0].Reason != ReasonUnreachable {
		t.Fatalf("reason = %q, want %q", res.Degraded[0].Reason, ReasonUnreachable)
	}
}

package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synodl/server/internal/source"
)

// fakeProvider spins up an httptest server that serves the captured JSON fixtures
// by path, points the driver at it, and returns a matching Config.
func fakeProvider(t *testing.T, handler http.HandlerFunc) (source.Config, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := apiBase
	apiBase = srv.URL
	cfg := source.Config{
		APIHosts:      []string{"127.0.0.1"},
		DownloadHosts: []string{"divyacamilla.info"},
	}
	return cfg, func() { apiBase = prev; srv.Close() }
}

func facetByValue(opts []source.FacetOption, value string) (source.FacetOption, bool) {
	for _, o := range opts {
		if o.Value == value {
			return o, true
		}
	}
	return source.FacetOption{}, false
}

func TestThirtynamaParameters(t *testing.T) {
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "advanced_search_parametres") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write(fixture(t, "advanced_search_parametres.json"))
	})
	defer done()

	p, err := nama30{}.Parameters(context.Background(), source.NewClient(), cfg, source.Session{})
	if err != nil {
		t.Fatalf("Parameters: %v", err)
	}

	// The empty "All" (value "") entry is dropped from every facet.
	if _, ok := facetByValue(p.Genres, ""); ok {
		t.Fatal("empty All entry should be dropped")
	}
	if len(p.Genres) != 2 {
		t.Fatalf("genres = %+v, want 2", p.Genres)
	}
	// Numeric values become their digits, slugs are preserved.
	if g, ok := facetByValue(p.Genres, "3373"); !ok || g.Slug != "sci-fi" {
		t.Fatalf("sci-fi genre = %+v", g)
	}
	// A compound type value survives verbatim.
	if _, ok := facetByValue(p.Types, "17&124913"); !ok {
		t.Fatalf("compound type value missing: %+v", p.Types)
	}
	// Float and negative score values keep their textual form.
	if _, ok := facetByValue(p.Scores, "8.5"); !ok {
		t.Fatalf("score 8.5 missing: %+v", p.Scores)
	}
	if _, ok := facetByValue(p.Scores, "-5"); !ok {
		t.Fatalf("score -5 missing: %+v", p.Scores)
	}
	// ISO country/language codes pass through for the client to localize.
	if _, ok := facetByValue(p.Countries, "US"); !ok {
		t.Fatalf("country US missing: %+v", p.Countries)
	}
	if _, ok := facetByValue(p.Languages, "en"); !ok {
		t.Fatalf("language en missing: %+v", p.Languages)
	}
	if p.MinYear != 1890 || p.MaxYear != 2026 {
		t.Fatalf("year bounds = %d..%d", p.MinYear, p.MaxYear)
	}
	// Channel is an object facet; encoder/age are plain string lists with blanks
	// dropped.
	if _, ok := facetByValue(p.Channels, "Netflix"); !ok {
		t.Fatalf("channel Netflix missing: %+v", p.Channels)
	}
	if len(p.Encoders) != 3 { // "30nama", "YIFY", "RARBG" — the "" is dropped
		t.Fatalf("encoders = %+v, want 3", p.Encoders)
	}
	if e, ok := facetByValue(p.Encoders, "YIFY"); !ok || e.Name != "YIFY" {
		t.Fatalf("encoder YIFY = %+v", e)
	}
	if _, ok := facetByValue(p.Ages, "PG-13"); !ok {
		t.Fatalf("age PG-13 missing: %+v", p.Ages)
	}
}

func TestThirtynamaBuildParamsAdvancedFacets(t *testing.T) {
	body := buildParams(source.SearchFilters{
		Channel: "Netflix", Encoder: "YIFY", X265: "true", ThreeD: "true",
		Cast: "brad pitt", Director: "nolan", Creator: "creator x", YearFrom: "2000", YearTo: "2010",
	})
	for _, want := range []string{
		`"channel":"Netflix"`, `"encoder":"YIFY"`, `"x265":"true"`, `"3d":"true"`,
		`"cast":"brad pitt"`, `"director":"nolan"`, `"creator":"creator x"`,
		`"min_year":"2000"`, `"max_year":"2010"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("params %s missing %s", body, want)
		}
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func defaultHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/download/"):
			w.Write(fixture(t, "download.json"))
		case strings.Contains(r.URL.Path, "advanced_search"), strings.Contains(r.URL.Path, "full_search"):
			w.Write(fixture(t, "advanced_search.json"))
		default:
			http.NotFound(w, r)
		}
	}
}

func TestThirtynamaSearchAdvanced(t *testing.T) {
	cfg, done := fakeProvider(t, defaultHandler(t))
	defer done()
	c := source.NewClient()

	res, err := nama30{}.Search(context.Background(), c, cfg, source.Session{},
		source.SearchQuery{Page: 1, Filters: source.SearchFilters{Type: "movie", Quality: "4K"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Page != 1 || res.Pages != 12 {
		t.Fatalf("pagination = %d/%d", res.Page, res.Pages)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(res.Items))
	}
	got := res.Items[0]
	if got.ID != "217561" || got.Title != "Soul 2020" || got.Type != "movie" {
		t.Fatalf("item0 = %+v", got)
	}
	if got.IMDbScore != 8 || got.ProviderScore != 8.8 {
		t.Fatalf("scores = %v / %v", got.IMDbScore, got.ProviderScore)
	}
	if got.PosterURL == "" || got.IMDbID != "tt2948372" {
		t.Fatalf("poster/imdb = %q / %q", got.PosterURL, got.IMDbID)
	}
	// Second item exercises string-typed score + null imdb + coming_soon.
	if !res.Items[1].ComingSoon || res.Items[1].ProviderScore != 7.5 {
		t.Fatalf("item1 = %+v", res.Items[1])
	}
}

func TestThirtynamaSearchQueryUsesFullSearch(t *testing.T) {
	var gotPath, gotBody string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()
	// A type filter must NOT reach the full_search path: the provider only accepts
	// type/all there (type/15 or type/movie return zero results).
	_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "soul", Page: 2, Filters: source.SearchFilters{Type: "movie"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotPath, "full_search") || !strings.Contains(gotPath, "/page/2") {
		t.Fatalf("path = %q, want full_search page 2", gotPath)
	}
	if !strings.Contains(gotPath, "/type/all/") {
		t.Fatalf("path = %q, want type/all even with a Type filter set", gotPath)
	}
	if gotBody != "query=soul" {
		t.Fatalf("body = %q, want query=soul", gotBody)
	}
}

// full_search only accepts type/all, so the Type filter is honored client-side by
// dropping posts whose title_type doesn't match the selected type. The filter
// value may be the friendly name ("movie") or the provider's numeric code ("15").
func TestThirtynamaFullSearchTypeFilterReFilters(t *testing.T) {
	body := `{"success":true,"result":{"title":{"page":1,"pages":3,"posts":[
	  {"id":1,"title_type":"movie","title":"Batman Begins 2005","image":{"cover":"https://x/a.jpg"}},
	  {"id":2,"title_type":"series","title":"Batwoman 2019","image":{"cover":"https://x/b.jpg"}},
	  {"id":3,"title_type":"movie","title":"The Batman 2022","image":{"cover":"https://x/c.jpg"}}
	]},"person":{},"news":{}}}`
	var gotPath string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { gotPath = r.URL.Path; w.Write([]byte(body)) })
	defer done()

	// Movies filter → type/all path, series dropped.
	res, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "batman", Filters: source.SearchFilters{Type: "movie"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotPath, "/type/all/") {
		t.Fatalf("path = %q, want type/all", gotPath)
	}
	if len(res.Items) != 2 {
		t.Fatalf("type re-filter: items = %d, want 2 movies", len(res.Items))
	}
	for _, it := range res.Items {
		if it.Type != "movie" {
			t.Fatalf("non-movie leaked past the type filter: %+v", it)
		}
	}

	// The provider's numeric type code must narrow the same way.
	res2, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "batman", Filters: source.SearchFilters{Type: "15"}})
	if err != nil {
		t.Fatalf("Search (code): %v", err)
	}
	if len(res2.Items) != 2 {
		t.Fatalf("numeric-code re-filter: items = %d, want 2", len(res2.Items))
	}

	// No type filter → all three types come back.
	res3, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "batman"})
	if err != nil {
		t.Fatalf("Search (no filter): %v", err)
	}
	if len(res3.Items) != 3 {
		t.Fatalf("no type filter: items = %d, want 3", len(res3.Items))
	}
}

// Regression: VerifySession must NOT probe full_search with a 1-char query (the
// provider rejects it as "empty" → looks like invalid_token). It must use
// advanced_search, which returns success for a valid session.
func TestThirtynamaVerifyUsesAdvancedSearch(t *testing.T) {
	var gotPath, gotBody string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()
	if err := (nama30{}).VerifySession(context.Background(), source.NewClient(), cfg, source.Session{}); err != nil {
		t.Fatalf("VerifySession: %v", err)
	}
	if !strings.Contains(gotPath, "advanced_search") {
		t.Fatalf("verify path = %q, want advanced_search", gotPath)
	}
	if strings.Contains(gotBody, "query=") {
		t.Fatalf("verify must not send a text query, body = %q", gotBody)
	}
}

// Regression: a 1-char (or empty) query browses via advanced_search rather than
// full_search, which would be rejected as an empty search.
func TestThirtynamaShortQueryBrowses(t *testing.T) {
	var gotPath string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()
	_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "a"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotPath, "advanced_search") {
		t.Fatalf("1-char query path = %q, want advanced_search", gotPath)
	}
}

// Regression: a post whose image.cover is `false` (the provider's "no poster")
// must not fail the whole page's parse or trip needs-refresh.
func TestThirtynamaToleratesLooseTypes(t *testing.T) {
	body := `{"success":true,"result":{"page":1,"pages":9,"posts":[
	  {"id":100,"title_type":"movie","title":"Has Poster","imdb_id":"tt1","imdb_score":"7.1","30nama_score":8,"image":{"cover":"https://cdn.30nama.com/cover/100.jpg","poster":{"medium":"https://cdn.30nama.com/poster/100-m.jpg"}}},
	  {"id":200,"title_type":"series","title":"No Poster","imdb_id":false,"imdb_score":false,"30nama_score":false,"image":{"cover":false}},
	  {"id":300,"title_type":"movie","title":"Coming Soon","coming_soon":true,"image":{"cover":false,"poster":{"big":"https://cdn.30nama.com/none/none-b_30NAMA.jpg?2","medium":"https://cdn.30nama.com/none/none-m_30NAMA.jpg?2"}}}
	]}}`
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	defer done()
	res, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, source.SearchQuery{})
	if err != nil {
		t.Fatalf("Search with a bool cover must not error: %v", err)
	}
	if len(res.Items) != 3 || res.Pages != 9 {
		t.Fatalf("got %d items, pages=%d", len(res.Items), res.Pages)
	}
	// A title with both images uses the portrait POSTER as the primary (thumbnail
	// + grid), the wide cover as the backdrop, and the cover as the load-fallback.
	if res.Items[0].PosterURL != "https://cdn.30nama.com/poster/100-m.jpg" ||
		res.Items[0].BackdropURL != "https://cdn.30nama.com/cover/100.jpg" ||
		res.Items[0].PosterFallbackURL != "https://cdn.30nama.com/cover/100.jpg" {
		t.Fatalf("poster/backdrop mapped wrong: %+v", res.Items[0])
	}
	if res.Items[1].PosterURL != "" || res.Items[1].IMDbID != "" || res.Items[1].Title != "No Poster" {
		t.Fatalf("coverless post mapped wrong: %+v", res.Items[1])
	}
	// A coming-soon title with no cover shows the provider's placeholder poster and
	// has no distinct backdrop.
	if res.Items[2].PosterURL != "https://cdn.30nama.com/none/none-m_30NAMA.jpg?2" ||
		res.Items[2].BackdropURL != "" {
		t.Fatalf("coming-soon poster/backdrop = %q / %q", res.Items[2].PosterURL, res.Items[2].BackdropURL)
	}
}

// Provider text arrives HTML-encoded (e.g. &#039; for an apostrophe, &amp; for
// &); it must be decoded so titles/plots read correctly, not literally.
func TestThirtynamaDecodesHTMLEntities(t *testing.T) {
	body := `{"success":true,"result":{"page":1,"pages":1,"posts":[
	  {"id":1,"title_type":"movie","title":"Tom &amp; Jerry","english_plot":"A boy&#039;s room.","image":{"cover":"https://x/c.jpg"}}
	]}}`
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	defer done()
	res, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, source.SearchQuery{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Items[0].Title != "Tom & Jerry" || res.Items[0].Plot != "A boy's room." {
		t.Fatalf("entities not decoded: title=%q plot=%q", res.Items[0].Title, res.Items[0].Plot)
	}
}

// Regression: full_search nests results under result.title.{page,pages,posts};
// a text query must read that shape (not return empty).
func TestThirtynamaFullSearchNestedShape(t *testing.T) {
	body := `{"success":true,"result":{"title":{"page":1,"pages":2,"posts":[
	  {"id":232162,"title_type":"movie","title":"The Matrix Resurrections 2021","imdb_id":"tt10838180","imdb_score":"5.6","30nama_score":5.4,"image":{"cover":"https://x/m.jpg"}}
	]},"person":{},"news":{}}}`
	var gotPath string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { gotPath = r.URL.Path; w.Write([]byte(body)) })
	defer done()
	res, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, source.SearchQuery{Query: "matrix"})
	if err != nil {
		t.Fatalf("full_search: %v", err)
	}
	if !strings.Contains(gotPath, "full_search") {
		t.Fatalf("path = %q", gotPath)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "The Matrix Resurrections 2021" || res.Pages != 2 {
		t.Fatalf("nested full_search parse: %+v (pages=%d)", res.Items, res.Pages)
	}
}

// Browse sends the provider's numeric type code (not the plain name, which the
// API silently ignores), the score/genre codes, and the chosen orderby;
// an empty sort defaults to release-year descending.
func TestThirtynamaBrowseFiltersAndSort(t *testing.T) {
	var gotPath, gotBody string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()

	_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Sort: "favorite", Filters: source.SearchFilters{
			Type: "series", Score: "8", Genre: []string{"3355"},
		}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotPath, "orderby/favorite") {
		t.Fatalf("path = %q, want orderby/favorite", gotPath)
	}
	// No explicit order → the provider default, descending.
	if !strings.Contains(gotPath, "order/desc") {
		t.Fatalf("path = %q, want order/desc by default", gotPath)
	}
	// Body is url-encoded: parameters=%7B...%7D
	dec, _ := url.QueryUnescape(gotBody)
	if !strings.Contains(dec, `"type":"16"`) {
		t.Fatalf("series must map to code 16; body = %q", dec)
	}
	if !strings.Contains(dec, `"score":"8"`) || !strings.Contains(dec, `"genre":["3355"]`) {
		t.Fatalf("score/genre missing; body = %q", dec)
	}

	// Empty sort → Most popular default (spec 2007); movie → code 15.
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Filters: source.SearchFilters{Type: "movie"}})
	dec2, _ := url.QueryUnescape(gotBody)
	if !strings.Contains(gotPath, "orderby/favorite") {
		t.Fatalf("default sort path = %q, want orderby/favorite", gotPath)
	}
	if !strings.Contains(dec2, `"type":"15"`) {
		t.Fatalf("movie must map to code 15; body = %q", dec2)
	}

	// An explicit ascending order reverses the browse.
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Sort: "favorite", Order: "asc"})
	if !strings.Contains(gotPath, "orderby/favorite/order/asc") {
		t.Fatalf("ascending path = %q, want orderby/favorite/order/asc", gotPath)
	}

	// A type value that's already a provider code (from the live facet list, e.g.
	// "15" for Movies) passes straight through — it must NOT be dropped or altered.
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Filters: source.SearchFilters{Type: "15"}})
	if dec, _ := url.QueryUnescape(gotBody); !strings.Contains(dec, `"type":"15"`) {
		t.Fatalf("numeric type code must pass through; body = %q", dec)
	}
}

// The release-year sort must send NO implicit year bounds. Spec 2006 added them
// to hide the source's ~350 broken-year rows; timing the live API afterwards
// showed the min_year filter makes their query 15-20s on any uncached page (vs
// 1.5-2s without it), so spec 2007 removed them. A year range the USER set is
// still sent, unchanged, under any sort.
func TestThirtynamaYearSortSendsNoImplicitBounds(t *testing.T) {
	var gotBody string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()
	body := func() string {
		dec, _ := url.QueryUnescape(gotBody)
		return dec
	}

	for _, q := range []source.SearchQuery{
		{Sort: "year", Order: "asc"},
		{Sort: "year", Order: "desc"},
		{Sort: "favorite"},
		{}, // the empty sort, whatever it resolves to
	} {
		_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, q)
		if err != nil {
			t.Fatalf("Search %+v: %v", q, err)
		}
		if strings.Contains(body(), "min_year") || strings.Contains(body(), "max_year") {
			t.Fatalf("sort %q must not add year bounds; body = %q", q.Sort, body())
		}
	}

	// A user-set range still reaches the provider untouched.
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Sort: "year", Filters: source.SearchFilters{YearFrom: "2000", YearTo: "2010"}})
	if !strings.Contains(body(), `"min_year":"2000"`) || !strings.Contains(body(), `"max_year":"2010"`) {
		t.Fatalf("user-set year range must survive; body = %q", body())
	}
}

// An empty or unrecognised sort resolves to Most popular — the same default the
// client uses (DEFAULT_SORT in useSourceCatalog.ts), so the two can't drift.
// It used to fall back to the release-year sort, which leads with the source's
// broken-year rows and is the worst thing to land on (spec 2007).
func TestThirtynamaDefaultSortIsFavorite(t *testing.T) {
	var gotPath string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()

	for _, sort := range []string{"", "nonsense"} {
		_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
			source.SearchQuery{Sort: sort})
		if err != nil {
			t.Fatalf("Search sort=%q: %v", sort, err)
		}
		if !strings.Contains(gotPath, "orderby/favorite/order/desc") {
			t.Fatalf("sort %q → path %q, want orderby/favorite/order/desc", sort, gotPath)
		}
	}
}

// A text search ALWAYS uses the full_search type/all path — with or without a
// type filter — because full_search rejects a numeric code (type/15) or slug
// (type/movie) with zero results. The type filter is applied to the results, not
// the path (see TestThirtynamaFullSearchTypeFilterReFilters).
func TestThirtynamaFullSearchTypeInPath(t *testing.T) {
	var gotPath string
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write(fixture(t, "advanced_search.json"))
	})
	defer done()

	// No type → type/all.
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "matrix"})
	if !strings.Contains(gotPath, "full_search/type/all/") {
		t.Fatalf("untyped text search path = %q, want type/all", gotPath)
	}
	// A type filter still uses type/all in the path (not type/16).
	_, _ = nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "matrix", Filters: source.SearchFilters{Type: "series"}})
	if !strings.Contains(gotPath, "full_search/type/all/") {
		t.Fatalf("typed text search path = %q, want type/all", gotPath)
	}
}

// A series' download returns season packs (is_series:true, entries use `link`
// and carry season info) — each becomes a sendable per-season quality option.
func TestThirtynamaSeriesDownload(t *testing.T) {
	// A series season pack's `link` is an array with one signed URL per episode.
	body := `{"success":true,"result":{"seasons":2,"is_series":true,"download":[
	  {"id":"s1","season_int":1,"season_name":"Season 1","total_episode":2,"quality":"1080p WEB-DL","size":"800 MB","encoder":"NTb","link":[
	    {"id":"e1","dl":"https://eu-download-storage-11.divyacamilla.info/download/s1e1"},
	    {"id":"e2","dl":"https://eu-download-storage-11.divyacamilla.info/download/s1e2"}]},
	  {"id":"s2","season_int":2,"season_name":"Season 2","total_episode":1,"quality":"1080p WEB-DL","size":"700 MB","encoder":"NTb","link":[
	    {"id":"e3","dl":"https://eu-download-storage-11.divyacamilla.info/download/s2e1"}]}
	]}}`
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	defer done()
	c := source.NewClient()

	td, err := nama30{}.Title(context.Background(), c, cfg, source.Session{}, "12157")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if td.Type != source.TypeSeries || !td.Sendable || len(td.Qualities) != 2 {
		t.Fatalf("series title = %+v", td)
	}
	if td.Qualities[0].Season != "Season 1" || td.Qualities[0].Episodes != 2 {
		t.Fatalf("season option = %+v", td.Qualities[0])
	}

	// Resolving season 1 yields one link per episode.
	links, size, err := nama30{}.ResolveDownload(context.Background(), c, cfg, source.Session{}, "12157", "s1")
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if len(links) != 2 || !strings.Contains(links[0], "s1e1") || !strings.Contains(links[1], "s1e2") || size != "800 MB" {
		t.Fatalf("resolved season links = %q size = %q", links, size)
	}
}

func TestThirtynamaTitleAndResolve(t *testing.T) {
	cfg, done := fakeProvider(t, defaultHandler(t))
	defer done()
	c := source.NewClient()

	td, err := nama30{}.Title(context.Background(), c, cfg, source.Session{}, "217561")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if !td.Sendable || len(td.Qualities) != 2 {
		t.Fatalf("title = %+v", td)
	}
	if td.Qualities[0].ID != "217561517290" || td.Qualities[0].Label != "x265 BluRay REMUX 2160p" ||
		td.Qualities[0].Size != "37.55 GB" || td.Qualities[0].Resolution != "3840x2160" {
		t.Fatalf("quality0 = %+v", td.Qualities[0])
	}
	// A QualityOption must never carry the signed URL.
	if strings.Contains(td.Qualities[0].Label, "divyacamilla") {
		t.Fatal("quality label leaked a URL")
	}

	links, size, err := nama30{}.ResolveDownload(context.Background(), c, cfg, source.Session{}, "217561", "217561512085")
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if len(links) != 1 || !strings.Contains(links[0], "512085") || !strings.Contains(links[0], "divyacamilla.info") {
		t.Fatalf("resolved links = %q", links)
	}
	if size != "11 GB" {
		t.Fatalf("resolved size = %q, want 11 GB", size)
	}

	// Unknown quality id.
	if _, _, err := (nama30{}).ResolveDownload(context.Background(), c, cfg, source.Session{}, "217561", "nope"); err == nil {
		t.Fatal("want error for unknown quality id")
	}
}

func TestThirtynamaResolveRejectsDisallowedDownloadHost(t *testing.T) {
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// dl points to a host NOT in DownloadHosts.
		w.Write([]byte(`{"success":true,"result":{"download":[{"id":"q1","quality":"x","dl":"https://evil.example.com/f"}]}}`))
	})
	defer done()
	_, _, err := nama30{}.ResolveDownload(context.Background(), source.NewClient(), cfg, source.Session{}, "1", "q1")
	if err != source.ErrHostNotAllowed {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

func TestThirtynamaVerifySession(t *testing.T) {
	// Success.
	cfg, done := fakeProvider(t, defaultHandler(t))
	if err := (nama30{}).VerifySession(context.Background(), source.NewClient(), cfg, source.Session{}); err != nil {
		t.Fatalf("VerifySession ok: %v", err)
	}
	done()

	// Origin 404 (unauth) → invalid_token.
	cfg2, done2 := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`<html>404 Not Found</html>`))
	})
	defer done2()
	err := nama30{}.VerifySession(context.Background(), source.NewClient(), cfg2, source.Session{})
	var ve *source.ErrProviderVerify
	if err == nil || !asVerify(err, &ve) || ve.Reason != "invalid_token" {
		t.Fatalf("verify err = %v, want ErrProviderVerify{invalid_token}", err)
	}
}

func TestThirtynamaSearchExpiredTokenNeedsRefresh(t *testing.T) {
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"msg":"unauthorized","result":null}`))
	})
	defer done()
	_, err := nama30{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "x"})
	nr, ok := source.AsNeedsRefresh(err)
	if !ok || nr.Layer != source.LayerToken {
		t.Fatalf("err = %v, want ErrNeedsRefresh{token}", err)
	}
}

func asVerify(err error, target **source.ErrProviderVerify) bool {
	if v, ok := err.(*source.ErrProviderVerify); ok {
		*target = v
		return true
	}
	return false
}

// Spec 2011: this provider returns an empty `resolution` on every download while
// naming it plainly in the quality label. An option with no resolution cannot be
// compared against what is on the NAS, so after ownership became per-release
// NOTHING was ever marked for this source — the season header said "on your NAS"
// and not one of its options said which.
func TestNama30FillsInAResolutionFromItsLabel(t *testing.T) {
	for _, tc := range []struct{ quality, want string }{
		{"BluRay 1080p", "1080p"},
		{"x265 BluRay 1080p", "1080p"},
		{"BluRay 720p", "720p"},
		{"WEB-DL 2160p", "2160p"},
		{"4K BluRay", "2160p"},
		// Nothing to go on: better empty than guessed, which is what keeps a
		// wrong option from being marked as one the user already has.
		{"DVDRip", ""},
		{"", ""},
	} {
		if got := source.ResolutionOf(tc.quality); got != tc.want {
			t.Errorf("ResolutionOf(%q) = %q, want %q", tc.quality, got, tc.want)
		}
	}
}

// An explicit resolution from the provider still wins over the label.
func TestExplicitResolutionWins(t *testing.T) {
	if got := firstNonEmptyStr("1080p", source.ResolutionOf("BluRay 720p")); got != "1080p" {
		t.Fatalf("got %q, want the provider's own value", got)
	}
	if got := firstNonEmptyStr("", source.ResolutionOf("BluRay 720p")); got != "720p" {
		t.Fatalf("got %q, want the label fallback", got)
	}
}

package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	res, err := thirtynama{}.Search(context.Background(), c, cfg, source.Session{},
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
	_, err := thirtynama{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
		source.SearchQuery{Query: "soul", Page: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(gotPath, "full_search") || !strings.Contains(gotPath, "/page/2") {
		t.Fatalf("path = %q, want full_search page 2", gotPath)
	}
	if gotBody != "query=soul" {
		t.Fatalf("body = %q, want query=soul", gotBody)
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
	if err := (thirtynama{}).VerifySession(context.Background(), source.NewClient(), cfg, source.Session{}); err != nil {
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
	_, err := thirtynama{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
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
	  {"id":100,"title_type":"movie","title":"Has Poster","imdb_id":"tt1","imdb_score":"7.1","30nama_score":8,"image":{"cover":"https://x/c.jpg"}},
	  {"id":200,"title_type":"series","title":"No Poster","imdb_id":false,"imdb_score":false,"30nama_score":false,"image":{"cover":false}}
	]}}`
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
	defer done()
	res, err := thirtynama{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, source.SearchQuery{})
	if err != nil {
		t.Fatalf("Search with a bool cover must not error: %v", err)
	}
	if len(res.Items) != 2 || res.Pages != 9 {
		t.Fatalf("got %d items, pages=%d", len(res.Items), res.Pages)
	}
	if res.Items[1].PosterURL != "" || res.Items[1].IMDbID != "" || res.Items[1].Title != "No Poster" {
		t.Fatalf("coverless post mapped wrong: %+v", res.Items[1])
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
	res, err := thirtynama{}.Search(context.Background(), source.NewClient(), cfg, source.Session{}, source.SearchQuery{Query: "matrix"})
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

func TestThirtynamaTitleAndResolve(t *testing.T) {
	cfg, done := fakeProvider(t, defaultHandler(t))
	defer done()
	c := source.NewClient()

	td, err := thirtynama{}.Title(context.Background(), c, cfg, source.Session{}, "217561")
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

	link, err := thirtynama{}.ResolveDownload(context.Background(), c, cfg, source.Session{}, "217561", "217561512085")
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if !strings.Contains(link, "512085") || !strings.Contains(link, "divyacamilla.info") {
		t.Fatalf("resolved link = %q", link)
	}

	// Unknown quality id.
	if _, err := (thirtynama{}).ResolveDownload(context.Background(), c, cfg, source.Session{}, "217561", "nope"); err == nil {
		t.Fatal("want error for unknown quality id")
	}
}

func TestThirtynamaResolveRejectsDisallowedDownloadHost(t *testing.T) {
	cfg, done := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		// dl points to a host NOT in DownloadHosts.
		w.Write([]byte(`{"success":true,"result":{"download":[{"id":"q1","quality":"x","dl":"https://evil.example.com/f"}]}}`))
	})
	defer done()
	_, err := thirtynama{}.ResolveDownload(context.Background(), source.NewClient(), cfg, source.Session{}, "1", "q1")
	if err != source.ErrHostNotAllowed {
		t.Fatalf("err = %v, want ErrHostNotAllowed", err)
	}
}

func TestThirtynamaVerifySession(t *testing.T) {
	// Success.
	cfg, done := fakeProvider(t, defaultHandler(t))
	if err := (thirtynama{}).VerifySession(context.Background(), source.NewClient(), cfg, source.Session{}); err != nil {
		t.Fatalf("VerifySession ok: %v", err)
	}
	done()

	// Origin 404 (unauth) → invalid_token.
	cfg2, done2 := fakeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`<html>404 Not Found</html>`))
	})
	defer done2()
	err := thirtynama{}.VerifySession(context.Background(), source.NewClient(), cfg2, source.Session{})
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
	_, err := thirtynama{}.Search(context.Background(), source.NewClient(), cfg, source.Session{},
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

package providers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"synodl/server/internal/source"
)

// zarSession builds pasted material for the fake site.
func zarSession(cookie string) source.Session {
	return source.Session{
		UserAgent: "TestAgent/1.0",
		Fields:    map[string]string{zarFieldCookie: cookie},
	}
}

// zarFakeSite serves the captured fixtures, and records what it was sent.
type zarFakeSite struct {
	*httptest.Server
	lastCookie string
	lastPath   string
	anonymous  bool // serve logged-out pages regardless of the cookie
	paywalled  bool // serve a page whose rows are all upsell links
	// meta names a metadata-block fixture to prepend to whatever page is served,
	// reproducing today's pages: the captured full pages predate the block, so
	// on their own they only exercise the "site publishes no synopsis" path.
	meta string
	// noForm makes the advanced-search form unobtainable, so the degrade path
	// is exercised rather than assumed.
	noForm bool
	// lastAjaxAction is what the driver last asked the site's AJAX endpoint for.
	lastAjaxAction string
	// loginPage serves what a MIRROR serves a session that is not valid on it: a
	// login form, status 200, and none of the markers a real page carries — not
	// even the logged-in flag. Nothing about it says "error".
	loginPage bool
	// emptyArchive serves a real, logged-in archive page that simply has no cards,
	// which is what asking for a page past the last one looks like.
	emptyArchive bool
}

// own makes a captured page link to THIS fake, the way a live site's pages link
// to the address that served them. The fixtures were captured on the site's old
// domain, which the driver no longer accepts links on (spec 1045).
func (site *zarFakeSite) own(page []byte) []byte {
	return bytes.ReplaceAll(page, []byte("https://zarfilm.com"), []byte(site.URL))
}

func newZarFakeSite(t *testing.T) *zarFakeSite {
	t.Helper()
	site := &zarFakeSite{}
	site.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.lastCookie = r.Header.Get("Cookie")
		site.lastPath = r.URL.RequestURI()
		// The site's search dialog, served the way the site serves it: a JSON
		// envelope from the WordPress AJAX endpoint (spec 1046).
		if r.Method == http.MethodPost && r.URL.Path == zarAdvancedFormPath {
			_ = r.ParseForm()
			site.lastAjaxAction = r.PostForm.Get("action")
			if site.noForm {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(mustFixture(t, "advanced_search_form.json"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if site.loginPage {
			// No ajax_var at all — exactly what the live mirror returns.
			w.Write([]byte(`<html><body><form action="/login"><input type="password" name="pwd"></form></body></html>`))
			return
		}
		if site.emptyArchive {
			w.Write([]byte(zarAjaxVarForTest(true) + `<div class="posts_hoder_archive"></div>`))
			return
		}
		if site.meta != "" {
			w.Write(site.own(mustFixture(t, site.meta)))
		}
		switch {
		case site.anonymous:
			w.Write(site.own(mustFixture(t, "logged_out.html")))
		case strings.Contains(r.URL.Path, "/series/the-loyalty-game"):
			w.Write(site.own(mustFixture(t, "series_subscribed.html")))
		case strings.Contains(r.URL.Path, "/all-movie/"), r.URL.Path == "/", strings.HasPrefix(r.URL.Path, "/page/"):
			// The archive: the genre routes the driver reads its slugs from, then
			// a page of cards — which is also what the advanced search and a text
			// search answer with.
			w.Write(site.own(mustFixture(t, "archive_filters.html")))
			w.Write(site.own(mustFixture(t, "archive_page1.html")))
		case site.paywalled:
			w.Write(site.own(mustFixture(t, "movie_unsubscribed.html")))
		default:
			w.Write(site.own(mustFixture(t, "movie_subscribed.html")))
		}
	}))
	t.Cleanup(site.Close)
	return site
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	return zarFixture(t, name)
}

// zarCfg points the driver at the fake the way an operator points it at the
// real site: by configuring its address (spec 1045 — there is no built-in one).
func zarCfg(site *zarFakeSite) source.Config {
	cfg := zarfilm{}.Hosts()
	cfg.MainBase = site.URL
	cfg.APIHosts = []string{"127.0.0.1"} // the httptest host
	return cfg
}

func TestZarfilmVerifySessionLoggedIn(t *testing.T) {
	site := newZarFakeSite(t)
	err := zarfilm{}.VerifySession(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"))
	if err != nil {
		t.Fatalf("verify with a good session: %v", err)
	}
}

// FR-019: not logged in and not entitled are different problems with different
// advice, and must not be reported as the same thing.
func TestZarfilmVerifySessionLoggedOut(t *testing.T) {
	site := newZarFakeSite(t)
	site.anonymous = true
	err := zarfilm{}.VerifySession(context.Background(), source.NewClient(), zarCfg(site), zarSession("stale"))
	var ve *source.ErrProviderVerify
	if !asVerifyErr(err, &ve) || ve.Reason != "invalid_token" {
		t.Fatalf("logged-out verify = %v, want invalid_token", err)
	}
}

func TestZarfilmVerifySessionUnsubscribed(t *testing.T) {
	site := newZarFakeSite(t)
	site.paywalled = true
	err := zarfilm{}.VerifySession(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"))
	var ve *source.ErrProviderVerify
	if !asVerifyErr(err, &ve) || ve.Reason != source.ReasonUnsubscribed {
		t.Fatalf("paywalled verify = %v, want unsubscribed (not a login failure)", err)
	}
}

func asVerifyErr(err error, target **source.ErrProviderVerify) bool {
	return asProviderVerifyErr(err, target)
}

func TestZarfilmSendsItsOwnCookieOnly(t *testing.T) {
	site := newZarFakeSite(t)
	// A session that also carries the OTHER provider's field names. None of them
	// may leave: a driver must send only what it declared.
	sess := source.Session{
		UserAgent: "TestAgent/1.0",
		Fields: map[string]string{
			zarFieldCookie: "MY-COOKIE",
			"cf_clearance": "OTHER-PROVIDER-CLEARANCE",
			"c_token":      "OTHER-PROVIDER-TOKEN",
		},
	}
	_, _ = zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site), sess, source.SearchQuery{Page: 1})
	if !strings.Contains(site.lastCookie, "MY-COOKIE") {
		t.Fatalf("own cookie not sent: %q", site.lastCookie)
	}
	for _, leak := range []string{"OTHER-PROVIDER-CLEARANCE", "OTHER-PROVIDER-TOKEN", "cf_clearance", "c_token"} {
		if strings.Contains(site.lastCookie, leak) {
			t.Fatalf("leaked another provider's material to this site: %q", site.lastCookie)
		}
	}
}

func TestZarfilmSearchBrowsesArchive(t *testing.T) {
	site := newZarFakeSite(t)
	res, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), source.SearchQuery{Page: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.HasPrefix(site.lastPath, "/page/2/?") || !strings.Contains(site.lastPath, "advsearch=on") {
		t.Fatalf("page 2 of the advanced search not requested, got %q", site.lastPath)
	}
	if len(res.Items) == 0 {
		t.Fatal("no items")
	}
	if res.Pages < 2 {
		t.Fatalf("pages = %d", res.Pages)
	}
	for _, it := range res.Items {
		if it.ID == "" || it.Title == "" || it.Type == "" {
			t.Fatalf("incomplete item: %+v", it)
		}
	}
}

func TestZarfilmSearchTextQuery(t *testing.T) {
	site := newZarFakeSite(t)
	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), source.SearchQuery{Query: "whisper", Page: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(site.lastPath, "s=whisper") {
		t.Fatalf("text search not issued, got %q", site.lastPath)
	}
}

func TestZarfilmTitleMovie(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "the-whisper-man-2026")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if td.Type != source.TypeMovie || !td.Sendable {
		t.Fatalf("unexpected detail: %+v", td)
	}
	if len(td.Qualities) == 0 {
		t.Fatal("no qualities")
	}
	for _, q := range td.Qualities {
		if q.Label == "" || q.Size == "" {
			t.Fatalf("quality missing label or size: %+v", q)
		}
	}
}

func TestZarfilmTitleSeries(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "series/the-loyalty-game")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if td.Type != source.TypeSeries {
		t.Fatalf("type = %q", td.Type)
	}
	if len(td.Qualities) == 0 {
		t.Fatal("no season options")
	}
	for _, q := range td.Qualities {
		if q.Season == "" || q.Episodes == 0 {
			t.Fatalf("season option missing season/episodes: %+v", q)
		}
	}
}

// Spec 1023: the sheet's header metadata comes from the catalog entry, and a
// ZarFilm catalog entry has none — so the title response has to carry it, for a
// movie and for a series alike.
func TestZarfilmTitleCarriesMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, meta, id, wantIMDb string
		wantType                 string
	}{
		{"movie", "movie_meta.html", "the-whisper-man-2026", "tt1756855", source.TypeMovie},
		{"series", "series_meta.html", "series/the-loyalty-game", "tt13210838", source.TypeSeries},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := newZarFakeSite(t)
			site.meta = tc.meta
			td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
				zarSession("abc"), tc.id)
			if err != nil {
				t.Fatalf("Title: %v", err)
			}
			if td.Type != tc.wantType {
				t.Fatalf("type = %q", td.Type)
			}
			if td.IMDbID != tc.wantIMDb {
				t.Fatalf("imdbId = %q, want %q", td.IMDbID, tc.wantIMDb)
			}
			if td.Plot == "" {
				t.Fatal("no plot")
			}
			// Metadata is an addition, not a replacement: the download options are
			// still what the caller asked for.
			if len(td.Qualities) == 0 {
				t.Fatal("no qualities")
			}
		})
	}
}

// A page carrying no metadata block still answers with its download options:
// missing metadata is never an error (FR-010).
func TestZarfilmTitleWithoutMetadataStillLists(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "the-whisper-man-2026")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if td.Plot != "" {
		t.Fatalf("plot = %q, want empty", td.Plot)
	}
	if len(td.Qualities) == 0 {
		t.Fatal("no qualities")
	}
}

// FR-022: links are fetched fresh at send time, and every one must be on the
// declared download host.
func TestZarfilmResolveDownload(t *testing.T) {
	site := newZarFakeSite(t)
	links, size, err := zarfilm{}.ResolveDownload(context.Background(), source.NewClient(),
		zarCfg(site), zarSession("abc"), "the-whisper-man-2026", "0")
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if len(links) != 1 || size == "" {
		t.Fatalf("links=%v size=%q", links, size)
	}
	if !strings.Contains(links[0], zarDownload) {
		t.Fatalf("link not on the declared download host: %q", links[0])
	}
}

func TestZarfilmResolveDownloadSeasonPack(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "series/the-loyalty-game")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	links, _, err := zarfilm{}.ResolveDownload(context.Background(), source.NewClient(),
		zarCfg(site), zarSession("abc"), "series/the-loyalty-game", td.Qualities[0].ID)
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if len(links) != td.Qualities[0].Episodes {
		t.Fatalf("got %d links, want %d episodes", len(links), td.Qualities[0].Episodes)
	}
}

// A crafted id must not be able to steer the driver off its own site.
func TestZarfilmRejectsHostileTitleIDs(t *testing.T) {
	site := newZarFakeSite(t)
	drv := zarfilm{}
	for _, bad := range []string{
		"https://evil.example/x", "//evil.example/x", "../../etc/passwd", "/absolute",
	} {
		_, err := drv.Title(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"), bad)
		if err == nil {
			t.Fatalf("Title accepted hostile id %q", bad)
		}
		_, _, err = drv.ResolveDownload(context.Background(), source.NewClient(),
			zarCfg(site), zarSession("abc"), bad, "0")
		if err == nil {
			t.Fatalf("ResolveDownload accepted hostile id %q", bad)
		}
	}
}

// FR-024: hosts are matched by domain suffix (the storage subdomain rotates),
// and the dns-prefetch hint on title pages is NOT the download host.
func TestZarfilmHostAllowlist(t *testing.T) {
	cfg := zarfilm{}.Hosts()
	for _, h := range []string{"dl6.indllserver.info", "dl7.indllserver.info", "indllserver.info"} {
		if !source.HostAllowed(h, cfg.DownloadHosts) {
			t.Fatalf("rotating storage subdomain %q should be allowed", h)
		}
	}
	for _, h := range []string{"zhomis.info", "evil.example", "indllserver.info.evil.example"} {
		if source.HostAllowed(h, cfg.DownloadHosts) {
			t.Fatalf("%q must not be an allowed download host", h)
		}
	}
	if source.HostAllowed("evil.example", cfg.APIHosts) {
		t.Fatal("api allowlist too wide")
	}
}

// Spec 1045. The site's old domain is gone from the driver entirely: nothing
// may be fetched from it, and no poster may be proxied from it, unless an
// operator configures it as the address.
func TestZarfilmHasNoBuiltInAddress(t *testing.T) {
	cfg := zarfilm{}.Hosts()
	for _, h := range []string{"zarfilm.com", "www.zarfilm.com", "zhomis.info"} {
		if source.HostAllowed(h, cfg.APIHosts) || source.HostAllowed(h, cfg.ImageHosts) {
			t.Fatalf("%q is reachable without being configured", h)
		}
	}
}

// With no address configured there is nowhere to go, and the driver says so
// rather than reaching for one of its own.
func TestZarfilmWithNoAddressIsUnavailable(t *testing.T) {
	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarfilm{}.Hosts(),
		zarSession("abc"), source.SearchQuery{Page: 1})
	if !source.IsUnavailable(err) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	if err := (zarfilm{}).VerifySession(context.Background(), source.NewClient(), zarfilm{}.Hosts(), zarSession("abc")); err == nil {
		t.Fatal("a source with no address verified")
	}
}

// Spec 1045. Verification checks the address the source is CONFIGURED with. It
// used to check the built-in domain whatever was configured, so a source working
// on its current address verified as unreachable while the old one was down.
func TestZarfilmVerifiesTheConfiguredAddress(t *testing.T) {
	site := newZarFakeSite(t)
	cfg := zarCfg(site)
	if err := (zarfilm{}).VerifySession(context.Background(), source.NewClient(), cfg, zarSession("abc")); err != nil {
		t.Fatalf("verify against the configured address: %v", err)
	}
}

// A session that has expired mid-use surfaces as needs-refresh, not as an empty
// catalog that would look like the site had no content.
func TestZarfilmExpiredSessionDuringBrowse(t *testing.T) {
	site := newZarFakeSite(t)
	site.anonymous = true
	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("stale"), source.SearchQuery{Page: 1})
	if _, ok := source.AsNeedsRefresh(err); !ok {
		t.Fatalf("expired browse = %v, want needs-refresh", err)
	}
}

// The pasted field is a cookie HEADER, not a value. WordPress names its login
// cookie `wordpress_logged_in_<per-install hash>`, so the value under a generic
// name authenticates as nobody — verified against the real site, where it comes
// back as an anonymous visitor and is indistinguishable from an expired session.
func TestZarfilmParsesAWholeCookieHeader(t *testing.T) {
	site := newZarFakeSite(t)
	sess := source.Session{
		UserAgent: "TestAgent/1.0",
		Fields: map[string]string{
			// Exactly what "Copy as cURL" yields, unrelated cookies and all.
			zarFieldCookie: "_ga=GA1.1.99; wordpress_logged_in_ab12cd=THE-VALUE; _lscache_vary=admin_bar%3A1; other=x",
		},
	}
	_, _ = zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site), sess, source.SearchQuery{Page: 1})

	// The hashed name is preserved — that is the whole point.
	if !strings.Contains(site.lastCookie, "wordpress_logged_in_ab12cd=THE-VALUE") {
		t.Fatalf("hashed login cookie name not preserved: %q", site.lastCookie)
	}
	// The cache-variant cookie rides along, so a cached anonymous page can't come
	// back and masquerade as an expired session.
	if !strings.Contains(site.lastCookie, "_lscache_vary=admin_bar%3A1") {
		t.Fatalf("_lscache_vary not forwarded: %q", site.lastCookie)
	}
	// Unrelated cookies from the paste are NOT forwarded — there is no reason to
	// send someone's analytics identifiers anywhere.
	for _, unrelated := range []string{"_ga", "other=x"} {
		if strings.Contains(site.lastCookie, unrelated) {
			t.Fatalf("forwarded an unrelated cookie %q: %q", unrelated, site.lastCookie)
		}
	}
}

// The two fields can still be filled separately, and the second one may carry
// the cache cookie on its own.
func TestZarfilmAcceptsCookiesSplitAcrossFields(t *testing.T) {
	site := newZarFakeSite(t)
	sess := source.Session{
		UserAgent: "TestAgent/1.0",
		Fields: map[string]string{
			zarFieldCookie: "wordpress_logged_in_ab12cd=THE-VALUE",
			zarFieldVary:   "_lscache_vary=admin_bar%3A1",
		},
	}
	_, _ = zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site), sess, source.SearchQuery{Page: 1})
	if !strings.Contains(site.lastCookie, "wordpress_logged_in_ab12cd=THE-VALUE") ||
		!strings.Contains(site.lastCookie, "_lscache_vary=admin_bar%3A1") {
		t.Fatalf("split fields not combined: %q", site.lastCookie)
	}
}

// Spec 1024 US1 / spec 1046: the driver declares what the site's advanced search
// can narrow by, read from the site's own search dialog.
func TestZarfilmDeclaresItsCapabilities(t *testing.T) {
	site := newZarFakeSite(t)
	params, err := zarfilm{}.Parameters(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"))
	if err != nil {
		t.Fatalf("Parameters: %v", err)
	}
	if site.lastAjaxAction != "get_advanced_search" {
		t.Fatalf("the form was asked for as %q", site.lastAjaxAction)
	}
	if len(params.Types) == 0 {
		t.Fatal("no types")
	}
	// Genres carry the site's own value (what the query parameter wants) AND an
	// English slug (what lets another source's genre join with it).
	var comedy *source.FacetOption
	for i, g := range params.Genres {
		if g.Slug == "comedy" {
			comedy = &params.Genres[i]
		}
	}
	if comedy == nil {
		t.Fatalf("no comedy genre among %d", len(params.Genres))
	}
	if comedy.Value == "comedy" {
		t.Fatal("genre value must be the site's own vocabulary, not the join slug")
	}
	// Sorts are declared in the CANONICAL vocabulary the client speaks, so the
	// same choice means the same thing whichever source honours it. The site has
	// no ordering by release year, so none is claimed.
	got := map[string]bool{}
	for _, so := range params.Sorts {
		got[so.Slug] = true
	}
	for _, want := range []string{"imdb", "date", "favorite"} {
		if !got[want] {
			t.Fatalf("sort %q not declared; have %+v", want, params.Sorts)
		}
	}
	if got["year"] {
		t.Fatalf("release-year ordering claimed, which the site cannot do: %+v", params.Sorts)
	}
	// Score bands carry a slug derived from their meaning, so "8 and above" from
	// one source joins with "8 and above" from another.
	var eight, under bool
	for _, sc := range params.Scores {
		if sc.Slug == "score-8" && sc.Value == "8" {
			eight = true
		}
		if sc.Slug == "score-under-5" {
			under = true
		}
	}
	if !eight || !under {
		t.Fatalf("score bands incomplete: %+v", params.Scores)
	}
	// Languages are the site's English names, each with the ISO code the other
	// source speaks; a name no code describes is offered without one.
	langs := map[string]string{}
	for _, l := range params.Languages {
		langs[l.Value] = l.Slug
	}
	if langs["Korean"] != "ko" || langs["Mandarin"] != "cmn" {
		t.Fatalf("languages = %v", langs)
	}
	if slug, offered := langs["کانتونی"]; !offered || slug != "" {
		t.Fatalf("a name without a code must still be offered, without one: %v", langs)
	}
	// Countries and qualities are the site's own words, verbatim.
	var japan, bluray bool
	for _, co := range params.Countries {
		if co.Value == "ژاپن" {
			japan = true
		}
	}
	for _, q := range params.Qualities {
		if q.Value == "BluRay 1080p" {
			bluray = true
		}
	}
	if !japan || !bluray {
		t.Fatalf("countries/qualities incomplete: %d / %d", len(params.Countries), len(params.Qualities))
	}
	if params.MinYear == 0 || params.MaxYear < 2026 {
		t.Fatalf("year range = %d–%d", params.MinYear, params.MaxYear)
	}
}

// A source that cannot report its abilities must still be usable: the browse goes
// on, the sheet simply offers less (FR-011).
func TestZarfilmCapabilitiesDegradeWhenTheFormIsUnobtainable(t *testing.T) {
	site := newZarFakeSite(t)
	site.noForm = true
	params, err := zarfilm{}.Parameters(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"))
	if err != nil {
		t.Fatalf("Parameters must not fail: %v", err)
	}
	if len(params.Genres) != 0 || len(params.Sorts) != 0 || len(params.Languages) != 0 {
		t.Fatalf("expected nothing declared, got %+v", params)
	}
	if len(params.Types) == 0 {
		t.Fatal("types are known without the form and must survive")
	}
}

// Spec 1046: everything the user chose reaches the site as the advanced-search
// form would send it — on the site root, with every field present. Asserted on
// the REQUEST the driver makes, because that is where a filter silently does
// nothing: a parameter the site does not read, or a range with one end missing.
func TestZarfilmSendsEveryChosenFilter(t *testing.T) {
	thisYear := strconv.Itoa(time.Now().Year())
	for _, tc := range []struct {
		name     string
		q        source.SearchQuery
		wantPath string
		want     map[string]string
		absent   []string
	}{
		{
			name:     "movie browse composes genre, score, ordering and pagination",
			q:        source.SearchQuery{Page: 2, Sort: "imdb", Filters: source.SearchFilters{Type: source.TypeMovie, Genre: []string{"کمدی"}, Score: "8"}},
			wantPath: "/page/2/",
			want: map[string]string{
				"advsearch": "on", "mobile_type": "post", "mobile_advsgenre": "کمدی", "search_order": "6",
				"minadvsimdbrate": "8", "maxadvsimdbrate": "10",
			},
		},
		{
			name:     "series browse switches the type and asks for newest",
			q:        source.SearchQuery{Page: 1, Sort: "date", Filters: source.SearchFilters{Type: source.TypeSeries}},
			wantPath: "/",
			want:     map[string]string{"mobile_type": "series", "search_order": "1", "mobile_advsgenre": "0"},
		},
		{
			name:     "language, country, quality and 3D travel verbatim",
			q:        source.SearchQuery{Page: 1, Filters: source.SearchFilters{Language: "Korean", Country: "ژاپن", Quality: "BluRay 1080p", ThreeD: "true"}},
			wantPath: "/",
			want:     map[string]string{"languageSearch": "Korean", "advscountry": "ژاپن", "advsqulity": "BluRay 1080p", "adv3D": "1", "adv4k": "0"},
		},
		{
			name:     "a below-5 band is sent as a range from zero",
			q:        source.SearchQuery{Page: 1, Filters: source.SearchFilters{Score: "-5"}},
			wantPath: "/",
			want:     map[string]string{"minadvsimdbrate": "0", "maxadvsimdbrate": "5"},
		},
		{
			name:     "a year range with one end is completed, because the site drops half a range",
			q:        source.SearchQuery{Page: 1, Filters: source.SearchFilters{YearFrom: "2015"}},
			wantPath: "/",
			want:     map[string]string{"yaersofmin": "2015", "yaersofmax": thisYear},
		},
		{
			name:     "no years chosen sends no year range",
			q:        source.SearchQuery{Page: 1},
			wantPath: "/",
			want:     map[string]string{"yaersofmin": "", "yaersofmax": "", "mobile_type": "all", "search_order": "0"},
		},
		{
			name:     "a text search is the plain search and carries nothing else",
			q:        source.SearchQuery{Page: 1, Query: "friends", Sort: "imdb", Filters: source.SearchFilters{Genre: []string{"کمدی"}}},
			wantPath: "/",
			want:     map[string]string{"s": "friends"},
			absent:   []string{"advsearch", "mobile_advsgenre", "search_order"},
		},
		{
			name:     "a later page of a text search",
			q:        source.SearchQuery{Page: 3, Query: "friends"},
			wantPath: "/page/3/",
			want:     map[string]string{"s": "friends"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := newZarFakeSite(t)
			if _, err := (zarfilm{}).Search(context.Background(), source.NewClient(), zarCfg(site), zarSession("abc"), tc.q); err != nil {
				t.Fatalf("Search: %v", err)
			}
			u, err := url.Parse(site.lastPath)
			if err != nil {
				t.Fatalf("parse %q: %v", site.lastPath, err)
			}
			if u.Path != tc.wantPath {
				t.Fatalf("path = %q, want %q", u.Path, tc.wantPath)
			}
			for k, v := range tc.want {
				if got := u.Query().Get(k); got != v {
					t.Fatalf("%s = %q, want %q (full: %s)", k, got, v, site.lastPath)
				}
			}
			for _, k := range tc.absent {
				if u.Query().Has(k) {
					t.Fatalf("%s must not be sent (full: %s)", k, site.lastPath)
				}
			}
		})
	}
}

// Spec 1026: a season option must say who encoded it, and must name the file it
// would produce — the site rewrites the release tokens inside its file names, so
// the file itself is the only thing that tells two releases apart.
func TestZarfilmSeasonOptionsCarryEncoderAndReleaseName(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "series/the-loyalty-game")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if len(td.Qualities) < 2 {
		t.Fatalf("expected several season options, got %d", len(td.Qualities))
	}
	names := map[string]bool{}
	for _, q := range td.Qualities {
		if q.Encoder == "" {
			t.Errorf("option %q names no encoder", q.Label)
		}
		if q.ReleaseName == "" {
			t.Fatalf("option %q names no file, so it can never be identified", q.Label)
		}
		// The encoder is shown separately, so it must not also be left in the label.
		if strings.Contains(q.Label, " - "+q.Encoder) {
			t.Errorf("option %q prints its encoder twice", q.Label)
		}
		names[q.ReleaseName] = true
	}
	// Distinct options must name distinct files, or nothing can tell them apart.
	if len(names) != len(td.Qualities) {
		t.Fatalf("%d options share only %d file names", len(td.Qualities), len(names))
	}
}

// The movie shape too: every option here carries the site's own name as its
// encoder, so the file is the only discriminator.
func TestZarfilmMovieOptionsNameTheirFile(t *testing.T) {
	site := newZarFakeSite(t)
	td, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "the-whisper-man-2026")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	names := map[string]bool{}
	for _, q := range td.Qualities {
		if q.ReleaseName == "" {
			t.Fatalf("option %q names no file", q.Label)
		}
		names[q.ReleaseName] = true
	}
	if len(names) != len(td.Qualities) {
		t.Fatalf("%d options share only %d file names", len(td.Qualities), len(names))
	}
}

// A paywalled row is an upsell, not a release: it must name no file, so it can
// never be mistaken for something already on the NAS.
//
// Asserted on the movie path because that is the one the fake site can actually
// serve unentitled — its series branch is a captured subscribed page, chosen
// before the paywall flag is consulted.
func TestZarfilmPaywalledRowsNameNoFile(t *testing.T) {
	site := newZarFakeSite(t)
	site.paywalled = true
	_, err := zarfilm{}.Title(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), "the-whisper-man-2026")
	// Every row being an upsell is an entitlement problem, which is what the
	// driver reports — there is no option left that could name a file.
	if !errors.Is(err, source.ErrUnsubscribed) {
		t.Fatalf("err = %v, want ErrUnsubscribed", err)
	}
}

// zarAjaxVarForTest mirrors the inline flag every real page of the site carries.
func zarAjaxVarForTest(loggedIn bool) string {
	u, logged := "0", ""
	if loggedIn {
		u, logged = "424242", "1"
	}
	return `<script>var ajax_var = {"ajaxurl":"/x","u":"` + u + `","logged":"` + logged + `"};</script>`
}

// Spec 2009: a session that is not valid on the host answering is told so with a
// LOGIN PAGE at status 200 — no cards, and none of the markers a real page has.
// Parsed naively that is indistinguishable from an empty archive, so the source
// reported success with no results and the app had nothing to say. It matters
// most on a mirror, where credentials captured on the main domain may simply not
// be valid.
func TestZarfilmLoginPageIsReportedAsANeedForRefreshing(t *testing.T) {
	site := newZarFakeSite(t)
	site.loginPage = true

	_, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), source.SearchQuery{Page: 1})

	var needs *source.ErrNeedsRefresh
	if !errors.As(err, &needs) {
		t.Fatalf("err = %v, want a needs-refresh so the user is told to re-paste", err)
	}
}

// The rule must stay narrow: running out of catalog is NOT a broken session.
func TestZarfilmEmptyArchiveIsStillJustEmpty(t *testing.T) {
	site := newZarFakeSite(t)
	site.emptyArchive = true

	res, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), source.SearchQuery{Page: 99})
	if err != nil {
		t.Fatalf("a logged-in page with no cards is an empty result, not an error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("items = %d, want none", len(res.Items))
	}
}

// Spec 1032: the archive cards carry a release year, and the driver used to
// parse it and then drop it on the floor when building the catalog title.
func TestZarfilmSearchCarriesReleaseYear(t *testing.T) {
	site := newZarFakeSite(t)
	res, err := zarfilm{}.Search(context.Background(), source.NewClient(), zarCfg(site),
		zarSession("abc"), source.SearchQuery{Page: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no items")
	}

	var withYear int
	for _, it := range res.Items {
		if it.Year == "" {
			continue
		}
		withYear++
		// A year the site publishes is a plain 4-digit one; anything else means
		// we picked up the wrong node.
		if len(it.Year) != 4 {
			t.Errorf("%q: year = %q, want a bare 4-digit year", it.Title, it.Year)
		}
		for _, r := range it.Year {
			if r < '0' || r > '9' {
				t.Errorf("%q: year = %q, want digits only", it.Title, it.Year)
				break
			}
		}
	}
	if withYear == 0 {
		t.Fatal("no item carried a release year; the archive fixture has them on every card")
	}
}

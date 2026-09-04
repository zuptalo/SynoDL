package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
}

func newZarFakeSite(t *testing.T) *zarFakeSite {
	t.Helper()
	site := &zarFakeSite{}
	site.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site.lastCookie = r.Header.Get("Cookie")
		site.lastPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if site.meta != "" {
			w.Write(mustFixture(t, site.meta))
		}
		switch {
		case site.anonymous:
			w.Write(mustFixture(t, "logged_out.html"))
		case strings.Contains(r.URL.Path, "/series/the-loyalty-game"):
			w.Write(mustFixture(t, "series_subscribed.html"))
		case strings.Contains(r.URL.Path, "/all-movie/"), r.URL.Path == "/":
			w.Write(mustFixture(t, "archive_page1.html"))
		case site.paywalled:
			w.Write(mustFixture(t, "movie_unsubscribed.html"))
		default:
			w.Write(mustFixture(t, "movie_subscribed.html"))
		}
	}))
	// Point the driver at the fake and allow its host.
	old := zarBase
	zarBase = site.URL
	t.Cleanup(func() { zarBase = old; site.Close() })
	return site
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	return zarFixture(t, name)
}

func zarCfg(site *zarFakeSite) source.Config {
	cfg := zarfilm{}.Hosts()
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
	if !strings.Contains(site.lastPath, "/all-movie/page/2/") {
		t.Fatalf("page 2 not requested, got %q", site.lastPath)
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

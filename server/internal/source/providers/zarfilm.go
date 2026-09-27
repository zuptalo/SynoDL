package providers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"synodl/server/internal/source"
)

// zarfilm drives the ZarFilm site (spec 0007), at whatever address the operator
// says it currently lives (spec 1045).
//
// It has NO built-in address. The site's original domain went down often enough
// to make the source unreliable, and the site keeps moving; an address compiled
// into a release is wrong on the site's schedule rather than ours. So the
// operator supplies the current one — today https://zhomis.info — together with
// the sign-in material issued BY that address, and the driver goes nowhere else.
//
// Unlike the other provider this site publishes no API: no REST endpoint, no
// JSON, no separate API host. Everything is read from the pages a browser gets,
// and authentication is a WordPress login cookie rather than a scoped token —
// which is why the driver builds its own request credentials instead of the
// shared client doing it (see source.Req.Cookies).
type zarfilm struct{}

func init() { source.Register(zarfilm{}) }

// zarDownload is the storage domain signed download links are served from. It
// does not depend on which address served the page (spec 1020), so it stays a
// fixed, provider-declared host even though the site's own address is not.
const zarDownload = "indllserver.info"

func (zarfilm) Kind() string { return "zarfilm" }

func (zarfilm) DisplayName() string { return "ZarFilm" }

// Session field keys.
const (
	zarFieldCookie = "wordpress_logged_in"
	zarFieldVary   = "lscache_vary"
)

// SessionFields declares what an operator pastes. The help text is where the
// elevated sensitivity of this material gets stated — at the point of paste,
// which is the only place it can actually inform the decision.
func (zarfilm) SessionFields() []source.SessionField {
	return []source.SessionField{
		{
			Key: zarFieldCookie, Label: "Cookie header", Secret: true, Required: true,
			Help: "Paste the WHOLE cookie line from a browser where you are signed in — " +
				"names included, e.g. \"wordpress_logged_in_ab12…=xyz; _lscache_vary=…\". " +
				"In Chrome, DevTools → Network → reload a page → right-click the request → " +
				"Copy as cURL, and take what follows -b. The names matter: the login " +
				"cookie's name carries a per-site hash, and the value on its own " +
				"authenticates as nobody. This is a FULL ACCOUNT credential — anyone " +
				"holding it can do anything your site account can, unlike a scoped API " +
				"token. Signed download links also carry your account id. Sign out at the " +
				"site to invalidate a cookie you have finished with.",
		},
		{
			Key: zarFieldVary, Label: "Extra cookies (optional)", Secret: true, Required: false,
			Help: "Only needed if the cookie line above did not already include " +
				"_lscache_vary. That cookie selects the logged-in cache variant; without " +
				"it the site can return a cached anonymous page, which looks exactly like " +
				"an expired session.",
		},
	}
}

// auth builds this driver's own credentials. They travel only to this site's
// hosts — the shared client no longer assembles auth for anybody.
//
// The pasted field is parsed as a COOKIE HEADER, not as a single value, because
// the name matters and an operator cannot reconstruct it. WordPress's login
// cookie is named `wordpress_logged_in_<per-install hash>`, and sending the
// value under a generic `wordpress_logged_in` authenticates as nobody — the site
// answers as an anonymous visitor, which is indistinguishable from an expired
// session unless you know to look for it.
//
// So a whole `Cookie:` blob can be pasted into one field (which is what "Copy as
// cURL" yields) and the cookies this driver needs are picked out of it.
func (zarfilm) auth(s source.Session) (headers, cookies map[string]string) {
	cookies = map[string]string{}
	collect := func(blob string) {
		for _, part := range strings.Split(blob, ";") {
			name, val, found := strings.Cut(strings.TrimSpace(part), "=")
			if !found {
				continue
			}
			name, val = strings.TrimSpace(name), strings.TrimSpace(val)
			// Only this site's own auth cookies are forwarded. A pasted blob can
			// carry analytics and other unrelated cookies; there is no reason to
			// send those anywhere.
			if strings.HasPrefix(name, "wordpress_logged_in") || strings.HasPrefix(name, "_lscache_vary") {
				cookies[name] = val
			}
		}
	}
	collect(s.Get(zarFieldCookie))
	collect(s.Get(zarFieldVary))

	// A value with no "=" at all can only be used under a guessed name. Real
	// sites reject that, but the in-repo fake accepts anything, so keep it
	// working rather than making the credential-free dev path a special case.
	if len(cookies) == 0 {
		if c := strings.TrimSpace(s.Get(zarFieldCookie)); c != "" {
			cookies["wordpress_logged_in"] = c
		}
	}
	return nil, cookies
}

// Hosts is the fixed outbound allowlist: the site itself, and the storage domain
// its signed links are served from. That domain rotates its subdomain
// (dl6., dl7., …) so it is allowed by domain suffix.
//
// Deliberately NOT included: the host this site dns-prefetches on title pages.
// It is a hint, not the download host, and allowlisting it would widen the
// outbound surface for nothing.
//
// The site's own host is NOT here: it is whatever the operator configured, and
// that address widens only this source's allowlist — and the image proxy's, for
// the posters the site serves itself — where the source is configured
// (spec 1045, 1020 FR-010).
func (zarfilm) Hosts() source.Config {
	cfg := source.Config{
		DownloadHosts: []string{zarDownload},
	}
	// Dev/e2e only, and only in a build made with the `sourcemock` tag: allow the
	// fake site's host so the driver can be exercised without real credentials.
	// A release build has no such branch at all.
	if b := mockBase("zarfilm"); b != "" {
		if h := hostOf(b); h != "" {
			cfg.APIHosts = append(cfg.APIHosts, h)
			cfg.DownloadHosts = append(cfg.DownloadHosts, h, "mockdl.invalid")
			cfg.ImageHosts = append(cfg.ImageHosts, h)
		}
	}
	return cfg
}

// RequiresAddress: there is no built-in address to fall back to (spec 1045).
// Except in a dev/e2e build pointed at the in-repo fake site, which stands in
// for the address so that path needs no configuration.
func (zarfilm) RequiresAddress() bool { return mockBase("zarfilm") == "" }

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// get fetches one page as a browser would.
// get fetches one page, falling back to the source's alternate domain when the
// preferred one is unavailable.
//
// Only an AVAILABILITY failure fails over. A logged-out or paywalled response is
// the site answering correctly, and a mirror would answer identically — retrying
// there would just double the work and report the wrong cause (FR-004).
func (p zarfilm) get(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path string) ([]byte, error) {
	var firstErr error
	for i, base := range p.bases(cfg) {
		body, err := p.getFrom(ctx, c, cfg, s, base, path)
		if err == nil {
			// Remember which address answered, so an outage does not make every
			// later request pay for a failed attempt first (FR-007).
			rememberWorkingBase(cfg, base)
			return body, nil
		}
		if !source.IsUnavailable(err) {
			return body, err
		}
		if i == 0 {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = &source.ErrUnavailable{Err: errors.New("no address configured")}
	}
	return nil, firstErr
}

// post sends a form-encoded request the way the site's own scripts do and
// returns the body, falling back between addresses exactly as get does. It is
// for the site's AJAX endpoints, which answer JSON — so the page's login marker
// is not looked for here; a session the site rejects answers 401/403 instead.
func (p zarfilm) post(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path, form string) ([]byte, error) {
	var firstErr error
	for i, base := range p.bases(cfg) {
		bs := cfg.SessionFor(base, s)
		headers, cookies := p.auth(bs)
		resp, err := c.Do(ctx, bs, cfg.APIHosts, source.Req{
			Method: "POST", URL: base + path, Body: form, XHR: true,
			Origin: base, Referer: base + "/",
			Headers: headers, Cookies: cookies,
		})
		if err == nil {
			switch {
			case resp.Status == 401 || resp.Status == 403:
				return nil, &source.ErrNeedsRefresh{Layer: source.LayerToken}
			case resp.Status != 200:
				return nil, fmt.Errorf("zarfilm: unexpected status %d", resp.Status)
			}
			rememberWorkingBase(cfg, base)
			return resp.Body, nil
		}
		if !source.IsUnavailable(err) {
			return nil, err
		}
		if i == 0 {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = &source.ErrUnavailable{Err: errors.New("no address configured")}
	}
	return nil, firstErr
}

// bases lists the addresses to try, preferred first. The main domain leads
// unless a recent success says the mirror is the one currently answering.
//
// The main address is the operator's (spec 1045). With none — a source saved
// before the address was required — an alternate one is used in its place, and
// with neither there is nowhere to go: the list is empty and every request
// fails as unavailable rather than guessing.
func (p zarfilm) bases(cfg source.Config) []string {
	primary := strings.TrimRight(strings.TrimSpace(cfg.MainBase), "/")
	alt := strings.TrimRight(strings.TrimSpace(cfg.AltBase), "/")
	if primary == "" {
		primary = mockBase("zarfilm")
	}
	if primary == "" {
		primary, alt = alt, ""
	}
	if primary == "" {
		return nil
	}
	// In a dev/e2e build the driver is pointed at a fake site; there is no mirror
	// of a fake, and adding one would only make those runs slower and stranger.
	if mockBase("zarfilm") != "" || alt == "" || alt == primary {
		return []string{primary}
	}
	if preferredBase(cfg) == alt {
		return []string{alt, primary}
	}
	return []string{primary, alt}
}

func (p zarfilm) getFrom(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, base, path string) ([]byte, error) {
	// Material belongs to an ADDRESS, not to the source: a challenge cookie is
	// issued per domain and a login cookie is tied to the address that issued it.
	// Sending the main address's material to the mirror is how an outage turned
	// into a catalog that answered every request with a login page (spec 0009).
	s = cfg.SessionFor(base, s)
	headers, cookies := p.auth(s)
	resp, err := c.Do(ctx, s, cfg.APIHosts, source.Req{
		Method:  "GET",
		URL:     base + path,
		Headers: headers,
		Cookies: cookies,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status == 401 || resp.Status == 403 {
		return nil, &source.ErrNeedsRefresh{Layer: source.LayerToken}
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("zarfilm: unexpected status %d", resp.Status)
	}
	// A page that comes back anonymous means the cookie is gone or was never
	// sent — the same condition as an expired session.
	if st := parseLoginState(resp.Body); !st.LoggedIn {
		return resp.Body, &source.ErrNeedsRefresh{Layer: source.LayerToken}
	}
	return resp.Body, nil
}

// VerifySession answers whether the pasted material works, and distinguishes the
// three outcomes that need different advice: not logged in (re-paste), logged in
// but not entitled to download (subscribe — re-pasting would not help), and
// unreachable.
//
// It checks the addresses the source is CONFIGURED with, in the same order a
// page fetch would, with each address's own material (spec 1045). It used to
// check the driver's built-in address whatever was configured — so with that
// domain down, a source working perfectly on its mirror was verified as
// unreachable and, after a few keep-alive probes, marked as needing a new
// sign-in.
func (p zarfilm) VerifySession(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) error {
	var resp *source.Resp
	var err error
	for _, base := range p.bases(cfg) {
		bs := cfg.SessionFor(base, s)
		headers, cookies := p.auth(bs)
		resp, err = c.Do(ctx, bs, cfg.APIHosts, source.Req{
			Method: "GET", URL: base + "/", Headers: headers, Cookies: cookies,
		})
		if err == nil || !source.IsUnavailable(err) {
			break
		}
	}
	if resp == nil && err == nil {
		// No address at all: nothing to verify against.
		return &source.ErrProviderVerify{Reason: source.ReasonUnreachable}
	}
	if err != nil {
		if _, ok := source.AsNeedsRefresh(err); ok {
			return &source.ErrProviderVerify{Reason: source.ReasonNeedsRefresh}
		}
		return &source.ErrProviderVerify{Reason: source.ReasonUnreachable}
	}
	if resp.Status != 200 {
		return &source.ErrProviderVerify{Reason: source.ReasonUnreachable}
	}
	if st := parseLoginState(resp.Body); !st.LoggedIn {
		return &source.ErrProviderVerify{Reason: "invalid_token"}
	}
	// Logged in. Entitlement is a SEPARATE question and needs a separate probe:
	// the login flag says nothing about whether the account can download, and
	// reporting an entitlement problem as a login problem would send the operator
	// round in circles re-pasting perfectly good material (FR-019).
	if entitled, err := p.probeEntitlement(ctx, c, cfg, s); err == nil && !entitled {
		return &source.ErrProviderVerify{Reason: source.ReasonUnsubscribed}
	}
	return nil
}

// probeEntitlement fetches one real title and looks at what its download rows
// are. A visitor without entitlement gets upsell links in place of real ones.
// An error here is inconclusive, not a failure: the caller treats "can't tell"
// as entitled rather than accusing the operator of not paying.
func (p zarfilm) probeEntitlement(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) (bool, error) {
	items, _, err := p.listing(ctx, c, cfg, s, "/all-movie/page/1/")
	if err != nil || len(items) == 0 {
		return true, err
	}
	body, err := p.get(ctx, c, cfg, s, "/"+items[0].ID+"/")
	if err != nil {
		return true, err
	}
	rows, err := parseTitlePage(body)
	if err != nil || len(rows) == 0 {
		return true, err
	}
	for _, r := range rows {
		if !r.Paywalled {
			return true, nil
		}
	}
	return false, nil
}

// listing fetches and parses one archive/search page.
func (p zarfilm) listing(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path string) ([]zarListItem, int, error) {
	body, err := p.get(ctx, c, cfg, s, path)
	if err != nil {
		return nil, 0, err
	}
	// Page links are absolute and name whichever host served them — the mirror's
	// pages link to the mirror. All the addresses this source may legitimately be
	// reached at are accepted; anything else is still off-site and rejected.
	items, err := parseListing(body, p.linkBases(cfg)...)
	if err != nil {
		return nil, 0, err
	}
	// A session that is not valid HERE is answered with a login page — status 200,
	// no cards. Parsed naively that is indistinguishable from "the archive is
	// empty", so the source reported success with no results and the app had
	// nothing to tell the user. It matters most on a mirror: credentials captured
	// on the main domain are not necessarily valid on the alternate one, and this
	// is the moment that becomes visible.
	//
	// Deliberately narrow: only when NOTHING parsed AND the page does not say we
	// are logged in. A real archive page past its last page is still a logged-in
	// page, so exhausting the catalog still reads as an empty result, not as a
	// broken session.
	if len(items) == 0 && !parseLoginState(body).LoggedIn {
		return nil, 0, &source.ErrNeedsRefresh{Layer: source.LayerToken}
	}
	return items, parsePageCount(body), nil
}

// Search browses the catalog through the site's advanced search, or runs a
// text search (spec 1046).
//
// Browsing used to walk the archive routes (/all-movie/, /series/) with the
// handful of query parameters the archive's filter panel offered. The redesign
// removed that panel, and the site's own advanced-search dialog turned out to
// express far more — type, genre, ordering, language, country, release quality,
// an IMDb range and a year range — as one GET against the site root. So that is
// what a browse is now: every page is /page/N/?advsearch=on&… with every field
// present, exactly as the dialog submits it.
//
// A text search is different in kind. It is the site's plain search, and it
// reads NOTHING else — verified live: every advanced field, and the ordering, is
// ignored beside `s`. So nothing else is sent with it, and only the type filter
// is applied, to what comes back.
func (p zarfilm) Search(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, q source.SearchQuery) (source.SearchResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	path := "/"
	if page > 1 {
		path = "/page/" + strconv.Itoa(page) + "/"
	}
	params := url.Values{}
	if query := strings.TrimSpace(q.Query); query != "" {
		params.Set("s", query)
	} else {
		params = zarAdvancedQuery(q)
	}
	path += "?" + params.Encode()

	items, pages, err := p.listing(ctx, c, cfg, s, path)
	if err != nil {
		return source.SearchResult{}, err
	}
	out := source.SearchResult{Page: page, Pages: pages}
	for _, it := range items {
		typ := source.TypeMovie
		if it.IsSeries {
			typ = source.TypeSeries
		}
		// The type is applied here as well as in the query: a text search cannot
		// express it at all, and a filter must mean what it says rather than being
		// silently ignored.
		if q.Filters.Type != "" && q.Filters.Type != typ {
			continue
		}
		out.Items = append(out.Items, source.CatalogTitle{
			ID:        it.ID,
			Type:      typ,
			Title:     it.Title,
			PosterURL: it.PosterURL,
			IMDbScore: it.Rating,
			// The card markup carries a release year, which was parsed here and
			// then discarded on the way out until spec 1032.
			Year:   it.Year,
			Genres: it.Genres,
		})
	}
	return out, nil
}

// zarMinYear is the earliest year the site's year range is asked for. The
// site's own dialog leaves both ends blank; a bound is only ever sent because
// the user set one, and then the OTHER end has to be sent too — the site
// silently drops a range with one end missing (verified live).
const zarMinYear = 1900

// zarAdvancedQuery is the advanced-search form, filled in. Every field the form
// has is sent, with its neutral value when the user chose nothing, because that
// is the request the site's own dialog makes and the one known to work.
func zarAdvancedQuery(q source.SearchQuery) url.Values {
	f := q.Filters
	v := url.Values{}
	v.Set("mobile_type", zarTypeParam(f.Type))
	v.Set("mobile_advsgenre", zarOr(firstNonEmpty(f.Genre), "0"))
	v.Set("search_order", zarOrderParam(q.Sort))
	v.Set("languageSearch", zarOr(strings.TrimSpace(f.Language), "0"))
	v.Set("advscountry", zarOr(strings.TrimSpace(f.Country), "0"))
	v.Set("advsqulity", strings.TrimSpace(f.Quality))
	v.Set("mobile_subtitle", "0")
	v.Set("mobile_dubled", "0")
	v.Set("mobile_online", "0")
	v.Set("adv3D", zarFlag(f.ThreeD))
	// The site's own 4K toggle returns nothing at all (verified live), so it is
	// never set: a filter that empties the grid is worse than none.
	v.Set("adv4k", "0")
	v.Set("imdbID", "")
	lo, hi := zarScoreRange(f.Score)
	v.Set("minadvsimdbrate", lo)
	v.Set("maxadvsimdbrate", hi)
	from, to := zarYearRange(f.YearFrom, f.YearTo)
	v.Set("yaersofmin", from)
	v.Set("yaersofmax", to)
	v.Set("advsearch", "on")
	return v
}

func zarOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func zarFlag(v string) string {
	if v == "true" || v == "1" {
		return "1"
	}
	return "0"
}

// zarTypeParam is the dialog's type switch: "post" is the site's word for a
// movie. A type it has no word for (anime) is asked for as everything, and the
// post-filter in Search then honours it by keeping nothing.
func zarTypeParam(t string) string {
	switch t {
	case source.TypeMovie:
		return "post"
	case source.TypeSeries:
		return "series"
	}
	return "all"
}

// zarOrderParam maps the canonical sort keys onto the site's ordering codes:
// 1 newest, 2 most viewed, 6 highest IMDb. "Most popular" is the site's view
// count — the nearest thing it has to the other source's favourites. The site
// cannot order by release year, and has no ascending order at all; an ordering
// it lacks leaves the default.
func zarOrderParam(sort string) string {
	switch sort {
	case "date":
		return "1"
	case "favorite":
		return "2"
	case "imdb":
		return "6"
	}
	return "0"
}

// zarOrderKey is the inverse: which canonical ordering a form code declares.
func zarOrderKey(code string) (key, label string) {
	switch code {
	case "1":
		return "date", "Recently added"
	case "2":
		return "favorite", "Most popular"
	case "6":
		return "imdb", "IMDb rating"
	}
	return "", ""
}

// zarScoreRange turns a score band into the range the site wants. It takes a
// minimum AND a maximum: a band with one end missing is silently dropped
// (verified live), so "8 and above" is sent as 8–10 and "below 5" as 0–5.
func zarScoreRange(band string) (lo, hi string) {
	switch band = strings.TrimSpace(band); {
	case band == "":
		return "", ""
	case strings.HasPrefix(band, "-"):
		return "0", strings.TrimPrefix(band, "-")
	default:
		return band, "10"
	}
}

// zarYearRange completes a half-open year range, for the same reason.
func zarYearRange(from, to string) (string, string) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" && to == "" {
		return "", ""
	}
	if from == "" {
		from = strconv.Itoa(zarMinYear)
	}
	if to == "" {
		to = strconv.Itoa(time.Now().Year())
	}
	return from, to
}

// firstNonEmpty returns the first usable entry of a multi-valued filter. The site
// takes one genre, so a caller asking for several gets the first honoured rather
// than all of them silently ignored.
func firstNonEmpty(vals []string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// Parameters reports what the site's advanced search can narrow by (spec 1046).
//
// The lists are read from the site's own search dialog, fetched the way the
// dialog fetches itself, so a genre or language the site adds appears here
// without a release. Each option's VALUE is what the query parameter takes —
// Persian for genres and countries, English for languages, the exact release
// label for qualities — and where another source names the same thing
// differently, a slug says so: the English genre slug from the archive's own
// routes, and an ISO code for a language.
//
// A form that cannot be fetched is not a failure of the source: the browse goes
// on and the sheet offers only what is known without it (FR-011).
func (p zarfilm) Parameters(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) (source.SearchParameters, error) {
	out := source.SearchParameters{
		Types: []source.FacetOption{
			{Value: source.TypeMovie, Slug: "movie", Name: "Movie"},
			{Value: source.TypeSeries, Slug: "series", Name: "Series"},
		},
		MinYear: zarMinYear,
		MaxYear: time.Now().Year(),
	}
	raw, err := p.post(ctx, c, cfg, s, zarAdvancedFormPath, zarAdvancedFormBody)
	if err != nil {
		return out, nil
	}
	form, err := parseAdvancedForm(raw)
	if err != nil {
		return out, nil
	}
	// The English slug for each genre lives in the archive's genre routes, not in
	// the form. Without them the genres are still offered — they just join with
	// no other source's.
	slugs := map[string]string{}
	if body, err := p.get(ctx, c, cfg, s, "/all-movie/"); err == nil {
		slugs = parseGenreSlugs(body)
	}
	for _, g := range form.Genres {
		out.Genres = append(out.Genres, source.FacetOption{Value: g.Value, Name: g.Label, Slug: slugs[g.Label]})
	}
	for _, l := range form.Languages {
		out.Languages = append(out.Languages, source.FacetOption{Value: l.Value, Name: l.Label, Slug: zarLanguageISO(l.Value)})
	}
	for _, co := range form.Countries {
		out.Countries = append(out.Countries, source.FacetOption{Value: co.Value, Name: co.Label})
	}
	for _, q := range form.Qualities {
		out.Qualities = append(out.Qualities, source.FacetOption{Value: q.Value, Name: q.Label})
	}
	out.Scores = zarScoreBands()
	out.Sorts = zarSorts(form.Orders)
	return out, nil
}

// The site's own search dialog is built server-side and fetched by its scripts
// from the WordPress AJAX endpoint. The same call, with no nonce, hands back the
// form as JSON.
const (
	zarAdvancedFormPath = "/wp-admin/admin-ajax.php"
	zarAdvancedFormBody = "action=get_advanced_search"
)

// zarScoreBands are the IMDb bands offered, in the same shape and with the same
// slugs as the other source's, so the two join. The site itself takes a range;
// zarScoreRange turns a band back into one.
func zarScoreBands() []source.FacetOption {
	out := []source.FacetOption{}
	for _, v := range []string{"9", "8.5", "8", "7.5", "7", "6.5", "6", "5"} {
		name := v + "+"
		if !strings.Contains(v, ".") {
			name = v + ".0+"
		}
		out = append(out, source.FacetOption{Value: v, Name: name, Slug: "score-" + v})
	}
	return append(out, source.FacetOption{Value: "-5", Name: "Under 5.0", Slug: "score-under-5"})
}

// zarSorts declares the orderings in the canonical vocabulary, in the order the
// other source lists them so the control reads the same whichever is browsed.
// Only codes the form actually offers are declared.
func zarSorts(orders []zarFacet) []source.FacetOption {
	offered := map[string]bool{}
	for _, o := range orders {
		offered[o.Value] = true
	}
	var out []source.FacetOption
	for _, code := range []string{"2", "6", "1"} {
		if !offered[code] {
			continue
		}
		key, label := zarOrderKey(code)
		out = append(out, source.FacetOption{Value: key, Slug: key, Name: label})
	}
	return out
}

// Title returns a title's downloadable options. Movies yield one option per
// release; series yield one per season/quality, matching the season-pack shape
// the catalog already uses.
func (p zarfilm) Title(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, id string) (source.TitleDetail, error) {
	if !source.ValidateTitleID(id) {
		return source.TitleDetail{}, errors.New("zarfilm: invalid title id")
	}
	body, err := p.get(ctx, c, cfg, s, "/"+id+"/")
	if err != nil {
		return source.TitleDetail{}, err
	}
	td := source.TitleDetail{ID: id, Sendable: true}
	// The site's listing pages carry no synopsis and no IMDb link, so a catalog
	// entry from a search has neither — but the page we have just fetched to read
	// the download options carries both. Take them from here rather than making a
	// second request, and take them for movies and series alike: the two page
	// types differ in how they present downloads, not in how they describe the
	// title. Either being absent is normal and never fails the request.
	td.IMDbID = parseIMDbID(body)
	td.Plot = parsePlot(body)
	// Who made it, out of the same page (spec 0014). Also free — the block sits
	// beside the synopsis we just read. The people arrive NAMED but not
	// identified: this site links to its own person pages rather than publishing
	// an IMDb id, so each carries a Ref the server resolves once through
	// ResolvePerson below and then remembers.
	p.applyCredits(&td, body, cfg)
	if strings.HasPrefix(id, "series/") {
		td.Type = source.TypeSeries
		seasons, err := parseSeriesPage(body)
		if err != nil {
			return source.TitleDetail{}, err
		}
		for i, sq := range seasons {
			if sq.Paywalled && len(sq.Episodes) == 0 {
				continue
			}
			// Who encoded it is named at the end of the quality, so it is lifted out
			// and shown the way a movie's encoder is — and dropped from the label,
			// which would otherwise print it twice (spec 1026 US2).
			encoder := qualityEncoder(sq.Quality)
			label := strings.TrimSpace(sq.Season + " " + qualityWithoutEncoder(sq.Quality))
			// The file this option produces: the site rewrites the release tokens
			// inside its file names, so the file itself is what identifies the
			// release. Any episode names it — the episode number is reduced out
			// when the two sides are compared.
			releaseName := ""
			if len(sq.Episodes) > 0 {
				releaseName = fileNameFromURL(sq.Episodes[0])
			}
			td.Qualities = append(td.Qualities, source.QualityOption{
				ID:          fmt.Sprintf("s%d-%d", sq.SeasonNum, i),
				Label:       label,
				Size:        sq.Size,
				Resolution:  sq.Resolution,
				Encoder:     encoder,
				Hardsub:     sq.Subtitle != "" && !sq.Dubbed,
				Season:      sq.Season,
				Episodes:    len(sq.Episodes),
				ReleaseName: releaseName,
			})
		}
		return td, nil
	}
	td.Type = source.TypeMovie
	rows, err := parseTitlePage(body)
	if err != nil {
		return source.TitleDetail{}, err
	}
	for i, r := range rows {
		if r.Paywalled {
			continue
		}
		td.Qualities = append(td.Qualities, source.QualityOption{
			ID:         strconv.Itoa(i),
			Label:      zarLabel(r),
			Size:       r.Size,
			Resolution: r.Resolution,
			Encoder:    r.Encoder,
			Hardsub:    r.Subtitle != "" && !r.Dubbed,
			// Every movie option here is labelled with the site's own name as its
			// encoder, so the file is the only thing that tells them apart.
			ReleaseName: fileNameFromURL(r.URL),
		})
	}
	// Rows existed but every one was a paywall: entitled callers see links, so
	// this is an entitlement problem, not an empty title.
	if len(td.Qualities) == 0 && len(rows) > 0 {
		return source.TitleDetail{}, source.ErrUnsubscribed
	}
	return td, nil
}

// zarLabel builds a human label for one movie release.
func zarLabel(r zarDownloadRow) string {
	parts := []string{}
	if r.Resolution != "" {
		parts = append(parts, r.Resolution)
	}
	if r.Dubbed {
		parts = append(parts, "Dubbed")
	} else if r.Subtitle != "" {
		parts = append(parts, r.Subtitle)
	}
	if r.Encoder != "" {
		parts = append(parts, r.Encoder)
	}
	if len(parts) == 0 {
		return "Download"
	}
	return strings.Join(parts, " · ")
}

// ResolveDownload re-fetches the title and returns fresh signed links.
//
// Links are never reused from when the title was viewed: they carry an expiry
// roughly a day out, and a stale one would fail on the NAS long after the user
// has stopped watching for it.
func (p zarfilm) ResolveDownload(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, titleID, qualityID string) ([]string, string, error) {
	if !source.ValidateTitleID(titleID) {
		return nil, "", errors.New("zarfilm: invalid title id")
	}
	body, err := p.get(ctx, c, cfg, s, "/"+titleID+"/")
	if err != nil {
		return nil, "", err
	}
	var links []string
	var size string

	if strings.HasPrefix(titleID, "series/") {
		seasons, err := parseSeriesPage(body)
		if err != nil {
			return nil, "", err
		}
		for i, sq := range seasons {
			if fmt.Sprintf("s%d-%d", sq.SeasonNum, i) != qualityID {
				continue
			}
			links, size = sq.Episodes, sq.Size
			break
		}
	} else {
		rows, err := parseTitlePage(body)
		if err != nil {
			return nil, "", err
		}
		idx, err := strconv.Atoi(qualityID)
		if err != nil || idx < 0 || idx >= len(rows) {
			return nil, "", errors.New("zarfilm: unknown quality")
		}
		if rows[idx].Paywalled {
			return nil, "", source.ErrUnsubscribed
		}
		links, size = []string{rows[idx].URL}, rows[idx].Size
	}

	if len(links) == 0 {
		return nil, "", errors.New("zarfilm: no link for that quality")
	}
	// Every link must be on the declared download host. A page could in principle
	// carry an off-site link; handing one to the NAS would take the download
	// outside the allowlist that bounds this whole feature.
	for _, l := range links {
		u, err := url.Parse(l)
		if err != nil || !source.HostAllowed(u.Hostname(), cfg.DownloadHosts) {
			return nil, "", source.ErrHostNotAllowed
		}
	}
	return links, size, nil
}

// asProviderVerifyErr extracts an *ErrProviderVerify from an error chain.
func asProviderVerifyErr(err error, target **source.ErrProviderVerify) bool {
	return errors.As(err, target)
}

// applyCredits maps the people named on a title page onto the shared shape.
//
// Cast first, then the crew roles, each kept apart and each capped. The site
// bills about three cast and one director, so the caps are never what limits
// what you see here — they exist so a redesign cannot turn one title into a
// hundred person-page fetches.
func (p zarfilm) applyCredits(td *source.TitleDetail, body []byte, cfg source.Config) {
	people := parseCredits(body, p.linkBases(cfg)...)
	if len(people) == 0 {
		return
	}
	byRole := map[string][]source.Person{}
	for _, e := range people {
		byRole[e.Role] = append(byRole[e.Role], source.Person{Name: e.Name, Ref: e.Ref})
	}
	td.Cast = source.ClampPeople(byRole[zarRoleCast], source.MaxCast)
	td.Directors = source.ClampPeople(byRole[zarRoleDirector], source.MaxCrew)
	td.Writers = source.ClampPeople(byRole[zarRoleWriter], source.MaxCrew)
}

// linkBases is every address a link on one of this site's pages might name: the
// addresses the source is configured with, and nothing else (spec 1045 — the
// old built-in domain is no longer one of them).
func (p zarfilm) linkBases(cfg source.Config) []string {
	return p.bases(cfg)
}

// ResolvePerson learns who somebody is from their own page on this site.
//
// This is the PersonResolver capability, and it exists because this site names
// people without identifying them. One request yields both halves — the IMDb id
// the tile links to, and a portrait on a host already allowlisted for this
// source's images — which is why it is worth making at all rather than going
// straight to the fallback with only a name, which could not be looked up
// reliably anyway (two people share a name far more often than an id).
//
// The call carries the operator's stored session, so it is made by the server
// while assembling a response it decided to assemble, never on a client's
// instruction (FR-014a). A failure returns an error and the caller carries on
// with the name alone.
func (p zarfilm) ResolvePerson(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, ref string) (source.Person, error) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if !zarPersonRef.MatchString(ref) {
		return source.Person{}, errors.New("zarfilm: not a person reference")
	}
	body, err := p.get(ctx, c, cfg, s, "/"+ref+"/")
	if err != nil {
		return source.Person{}, err
	}
	imdbID, photo := parsePersonPage(body)
	return source.Person{IMDbID: imdbID, PhotoURL: photo, Ref: ref}, nil
}

// zarPersonRef is the shape of a person's path on this site. Anchored, and
// narrow: this value becomes a URL path on a host we hold credentials for, so
// it is validated rather than trusted even though it came from the site's own
// markup a moment ago.
var zarPersonRef = regexp.MustCompile(`^(actor|director|writer)/[A-Za-z0-9%._~-]{1,200}$`)

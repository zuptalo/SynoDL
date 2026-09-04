package providers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"synodl/server/internal/source"
)

// zarfilm drives zarfilm.com (spec 0007).
//
// Unlike the other provider this site publishes no API: no REST endpoint, no
// JSON, no separate API host. Everything is read from the pages a browser gets,
// and authentication is a WordPress login cookie rather than a scoped token —
// which is why the driver builds its own request credentials instead of the
// shared client doing it (see source.Req.Cookies).
type zarfilm struct{}

func init() { source.Register(zarfilm{}) }

const (
	zarHost     = "zarfilm.com"
	zarDownload = "indllserver.info"
)

// zarBase is the site root. A var only so tests can point the driver at an
// httptest server; production always uses the real host.
var zarBase = "https://" + zarHost

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
func (zarfilm) Hosts() source.Config {
	cfg := source.Config{
		APIHosts:      []string{zarHost},
		DownloadHosts: []string{zarDownload},
		ImageHosts:    []string{zarHost}, // posters are served from the site itself
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

// DefaultAltBase is the mirror this driver currently knows about, offered to the
// administrator as a starting value. It is only a default: the site changes its
// mirror on its own schedule, so the operator can replace it without waiting for
// a release (FR-002).
func (zarfilm) DefaultAltBase() string { return "https://zhomis.info" }

// base is where this driver's requests go: the real site, or a fake one in a
// dev/e2e build.
func (zarfilm) base() string {
	if b := mockBase("zarfilm"); b != "" {
		return b
	}
	return zarBase
}

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

// bases lists the addresses to try, preferred first. The main domain leads
// unless a recent success says the mirror is the one currently answering.
func (p zarfilm) bases(cfg source.Config) []string {
	primary := p.base()
	alt := strings.TrimRight(strings.TrimSpace(cfg.AltBase), "/")
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
func (p zarfilm) VerifySession(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) error {
	headers, cookies := p.auth(s)
	resp, err := c.Do(ctx, s, cfg.APIHosts, source.Req{
		Method: "GET", URL: p.base() + "/", Headers: headers, Cookies: cookies,
	})
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
	bases := append(p.bases(cfg), zarBase, "https://"+zarHost)
	items, err := parseListing(body, bases...)
	if err != nil {
		return nil, 0, err
	}
	return items, parsePageCount(body), nil
}

// Search browses the archive or runs a text search.
//
// Browsing uses the plain paginated URLs rather than the site's own ajax
// pagination endpoint: that endpoint requires a nonce (it answers 403 without
// one), and the paginated URLs need no nonce and no XHR headers. Fewer moving
// parts, one fewer thing to break.
func (p zarfilm) Search(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, q source.SearchQuery) (source.SearchResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	// The route selects WHAT is being browsed; everything the user narrowed by
	// rides in the query string. Genre used to take over the route
	// (/genre/<slug>/), which meant a genre could never be combined with the
	// series listing and never composed with a sort — and the route's English
	// slugs do not exist for series at all.
	query := strings.TrimSpace(q.Query)
	var path string
	params := url.Values{}
	switch {
	case query != "":
		path = "/page/" + strconv.Itoa(page) + "/"
		params.Set("s", query)
	case q.Filters.Type == source.TypeSeries:
		path = "/series/page/" + strconv.Itoa(page) + "/"
	default:
		path = "/all-movie/page/" + strconv.Itoa(page) + "/"
	}
	if g := firstNonEmpty(q.Filters.Genre); g != "" {
		params.Set("filter_genre", g)
	}
	if sc := strings.TrimSpace(q.Filters.Score); sc != "" {
		params.Set("imdb_rate", sc)
	}
	// The site ignores an ordering on a text search. Offering one anyway would be
	// the "filter that silently does nothing" this driver's own facets avoid, so
	// it is simply not sent — the sheet disables the control in that mode too.
	if query == "" {
		if f := zarSortParam(q.Sort); f != "" {
			params.Set("sortby", f)
		}
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

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
		// A type filter the site can't express in the URL is applied here, so the
		// filter means what it says rather than being silently ignored.
		if q.Filters.Type != "" && q.Filters.Type != typ {
			continue
		}
		out.Items = append(out.Items, source.CatalogTitle{
			ID:        it.ID,
			Type:      typ,
			Title:     it.Title,
			PosterURL: it.PosterURL,
			IMDbScore: it.Rating,
			Genres:    it.Genres,
		})
	}
	return out, nil
}

// zarSortParam maps a canonical sort key onto the site's own ordering keyword —
// the inverse of zarSortKey, which is how those keys were declared in the first
// place. "" means "leave the site's default alone".
func zarSortParam(sort string) string {
	switch sort {
	case "date", "newest":
		return "newest"
	case "favorite", "popular":
		return "popular"
	case "imdb", "imdb_rate", "score":
		return "imdb_rate"
	case "year", "release":
		return "release"
	case "modified", "updated":
		return "modified"
	}
	return ""
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

// Parameters reports the facets this source can actually filter on. It is
// deliberately modest: the site's archive URLs express genre and type, and
// claiming more would produce filters that silently do nothing.
func (p zarfilm) Parameters(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) (source.SearchParameters, error) {
	out := source.SearchParameters{
		Types: []source.FacetOption{
			{Value: source.TypeMovie, Slug: "movie", Name: "Movie"},
			{Value: source.TypeSeries, Slug: "series", Name: "Series"},
		},
	}
	// The abilities are published on the archive pages themselves, so reading them
	// costs one page fetch. A failure here is NOT a failure of the source: the user
	// can still browse, they are simply offered fewer ways to narrow it (FR-011).
	body, err := p.get(ctx, c, cfg, s, "/all-movie/")
	if err != nil {
		return out, nil
	}
	panel := parseFilterPanel(body)
	slugs := parseGenreSlugs(body)

	for _, g := range panel.Genres {
		// Value stays the site's own (Persian) vocabulary, because that is what its
		// query parameter accepts. Slug is the English name the same page uses in
		// its own genre routes: it is what lets this genre join with another
		// source's, and what the client title-cases for display.
		out.Genres = append(out.Genres, source.FacetOption{
			Value: g.Value, Name: g.Label, Slug: slugs[g.Label],
		})
	}
	for _, sc := range panel.Scores {
		out.Scores = append(out.Scores, source.FacetOption{
			Value: sc.Value, Name: zarScoreName(sc.Value), Slug: zarScoreSlug(sc.Value),
		})
	}
	for _, so := range panel.Sorts {
		key, label := zarSortKey(so.Value)
		if key == "" {
			continue // an ordering we have no canonical name for
		}
		out.Sorts = append(out.Sorts, source.FacetOption{Value: key, Slug: key, Name: label})
	}
	return out, nil
}

// zarScoreSlug gives a score band an identity derived from its MEANING, so
// "8 and above" from this source joins with "8 and above" from another.
//
// The site's lowest band is the odd one out: its value is 4 but it means "below
// 5", not "4 and above". It gets an identity of its own so it can never be
// mistaken for another source's 4+ band — it stays available when this source is
// browsed alone, and drops out of any combined set.
func zarScoreSlug(v string) string {
	if v == "4" {
		return "score-under-5"
	}
	return "score-" + v
}

func zarScoreName(v string) string {
	if v == "4" {
		return "Under 5.0"
	}
	return v + ".0+"
}

// zarSortKey maps one of the site's ordering keywords onto the canonical sort
// vocabulary the client speaks, so choosing "IMDb rating" means the same thing
// whichever source honours it. "modified" has no canonical equivalent — no other
// source can order by "recently updated" — so it keeps its own key and therefore
// appears only when this source is browsed alone.
func zarSortKey(siteValue string) (key, label string) {
	switch siteValue {
	case "newest":
		return "date", "Recently added"
	case "popular":
		return "favorite", "Most popular"
	case "imdb_rate":
		return "imdb", "IMDb rating"
	case "release":
		return "year", "Release year"
	case "modified":
		return "modified", "Recently updated"
	}
	return "", ""
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
			td.Qualities = append(td.Qualities, source.QualityOption{
				ID:         fmt.Sprintf("s%d-%d", sq.SeasonNum, i),
				Label:      strings.TrimSpace(sq.Season + " " + sq.Quality),
				Size:       sq.Size,
				Resolution: sq.Resolution,
				Hardsub:    sq.Subtitle != "" && !sq.Dubbed,
				Season:     sq.Season,
				Episodes:   len(sq.Episodes),
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

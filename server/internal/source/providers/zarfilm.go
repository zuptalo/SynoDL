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
			Key: zarFieldCookie, Label: "Login cookie", Secret: true, Required: true,
			Help: "The wordpress_logged_in_* cookie value from a browser where you are " +
				"signed in. This is a FULL ACCOUNT credential — anyone holding it can do " +
				"anything your site account can, unlike a scoped API token. Signed download " +
				"links also carry your account id. Sign out at the site to invalidate a " +
				"cookie you have finished with.",
		},
		{
			Key: zarFieldVary, Label: "_lscache_vary cookie", Secret: true, Required: false,
			Help: "Selects the logged-in cache variant. Without it a cached anonymous page " +
				"can come back and look like an expired session.",
		},
	}
}

// auth builds this driver's own credentials. They travel only to this site's
// hosts — the shared client no longer assembles auth for anybody.
func (zarfilm) auth(s source.Session) (headers, cookies map[string]string) {
	cookies = map[string]string{}
	if c := s.Get(zarFieldCookie); c != "" {
		// Accept either a bare value or a full "name=value" paste, since the
		// cookie's name carries a per-install hash an operator can't be expected to
		// strip by hand.
		if name, val, found := strings.Cut(c, "="); found && strings.HasPrefix(name, "wordpress_logged_in") {
			cookies[strings.TrimSpace(name)] = strings.TrimSpace(val)
		} else {
			cookies["wordpress_logged_in"] = c
		}
	}
	if v := s.Get(zarFieldVary); v != "" {
		if name, val, found := strings.Cut(v, "="); found && strings.HasPrefix(name, "_lscache_vary") {
			cookies[strings.TrimSpace(name)] = strings.TrimSpace(val)
		} else {
			cookies["_lscache_vary"] = v
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
func (p zarfilm) get(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path string) ([]byte, error) {
	headers, cookies := p.auth(s)
	resp, err := c.Do(ctx, s, cfg.APIHosts, source.Req{
		Method:  "GET",
		URL:     p.base() + path,
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
	// Page links are absolute and always name the canonical host, whatever address
	// this driver used to reach the page.
	items, err := parseListing(body, p.base(), zarBase, "https://"+zarHost)
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
	var path string
	query := strings.TrimSpace(q.Query)
	switch {
	case query != "":
		path = "/page/" + strconv.Itoa(page) + "/?s=" + url.QueryEscape(query)
	case q.Filters.Genre != nil && len(q.Filters.Genre) > 0 && q.Filters.Genre[0] != "":
		path = "/genre/" + url.PathEscape(q.Filters.Genre[0]) + "/page/" + strconv.Itoa(page) + "/"
	case q.Filters.Type == source.TypeSeries:
		path = "/series/page/" + strconv.Itoa(page) + "/"
	default:
		path = "/all-movie/page/" + strconv.Itoa(page) + "/"
	}
	if f := zarSortParam(q.Sort); f != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + "filter=" + f
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

// zarSortParam maps a generic sort onto the site's own archive filter values.
func zarSortParam(sort string) string {
	switch sort {
	case "", "newest", "date":
		return ""
	case "imdb", "imdb_rate", "score":
		return "imdb_rate"
	case "modified", "updated":
		return "modified"
	}
	return ""
}

// Parameters reports the facets this source can actually filter on. It is
// deliberately modest: the site's archive URLs express genre and type, and
// claiming more would produce filters that silently do nothing.
func (p zarfilm) Parameters(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) (source.SearchParameters, error) {
	return source.SearchParameters{
		Types: []source.FacetOption{
			{Value: source.TypeMovie, Slug: "movie", Name: "Movie"},
			{Value: source.TypeSeries, Slug: "series", Name: "Series"},
		},
	}, nil
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

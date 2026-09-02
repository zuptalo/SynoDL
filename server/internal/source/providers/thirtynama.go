// Package providers holds concrete source-provider drivers. Each maps SynoDL's
// generic catalog operations onto one site's API. Drivers are registered by kind
// so the core stays provider-neutral (spec 0005).
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"synodl/server/internal/source"
)

// thirtynama drives the first supported provider. Its API lives on
// interface.30nama.com and authenticates via c-* headers + a Cloudflare
// clearance cookie (carried by source.Client). Requests originate from the
// 30nama.com web origin.
type thirtynama struct{}

func init() { source.Register(thirtynama{}) }

const (
	tnAPIHost = "interface.30nama.com"
	tnOrigin  = "https://30nama.com"
	tnReferer = "https://30nama.com/"
)

// apiBase is the API root. It is a var (not a const) solely so tests can point
// the driver at an httptest server; production always uses the real host.
var apiBase = "https://" + tnAPIHost

// errUnauth is the internal signal that the API rejected us as unauthenticated
// (origin 404 / non-JSON / success:false). Callers translate it to the right
// public error depending on context (verify vs. runtime).
var errUnauth = errors.New("thirtynama: unauthenticated")

// errBadShape is a parse/shape mismatch on an AUTHENTICATED (success:true)
// response. It is deliberately NOT errUnauth: a malformed field must never be
// reported as an expired session (which would tell the admin to re-paste for no
// reason). It surfaces as a generic provider error instead.
var errBadShape = errors.New("thirtynama: unexpected response shape")

func (thirtynama) Kind() string { return "thirtynama" }

func (thirtynama) DisplayName() string { return "30nama" }

// Session field keys. These match the JSON names the store has always sealed, so
// an existing installation's pasted material migrates into the field bag by key
// with nothing to re-enter.
const (
	tnFieldClearance  = "cf_clearance"
	tnFieldAPIKey     = "c_api_key"
	tnFieldToken      = "c_token"
	tnFieldPlatform   = "c_platform"
	tnFieldAppVersion = "c_app_version"
)

// SessionFields declares what an admin must paste. The admin form is generated
// from this, so the client needs no per-provider knowledge.
func (thirtynama) SessionFields() []source.SessionField {
	return []source.SessionField{
		{Key: tnFieldClearance, Label: "cf_clearance cookie", Secret: true, Required: true,
			Help: "From your browser's cookies for 30nama.com. Tied to the public address it was issued to."},
		{Key: tnFieldToken, Label: "c-token", Secret: true, Required: true,
			Help: "Scoped API token; authorizes catalog calls only."},
		{Key: tnFieldAPIKey, Label: "c-api-key", Secret: true, Required: true},
		{Key: tnFieldPlatform, Label: "c-platform (optional)", Secret: false},
		{Key: tnFieldAppVersion, Label: "c-app-version (optional)", Secret: false},
	}
}

// auth builds this driver's own request credentials. It lives here, not in the
// shared client, so 30nama's material is only ever sent to 30nama's hosts — with
// a second source configured, a client that set these headers on every call
// would have handed them to the other site.
func (thirtynama) auth(s source.Session) (headers, cookies map[string]string) {
	headers = map[string]string{
		"c-api-key":     s.Get(tnFieldAPIKey),
		"c-token":       s.Get(tnFieldToken),
		"c-platform":    s.Get(tnFieldPlatform),
		"c-app-version": s.Get(tnFieldAppVersion),
		"c-useragent":   s.UserAgent,
	}
	cookies = map[string]string{"cf_clearance": s.Get(tnFieldClearance)}
	return headers, cookies
}

// Hosts is the provider's fixed outbound allowlist: the API host plus the site
// (for clearance verification) and the signed-download storage domain. The
// download storage host rotates its subdomain (eu-download-storage-NN.*), so it
// is allowed by domain suffix.
func (thirtynama) Hosts() source.Config {
	return source.Config{
		APIHosts:      []string{tnAPIHost, "30nama.com"},
		DownloadHosts: []string{"divyacamilla.info"},
		ImageHosts:    []string{"cdn.30nama.com"},
	}
}

func (p thirtynama) VerifySession(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) error {
	// Cheapest authenticated call: an empty advanced_search (parameters={}).
	// NOT full_search — the provider rejects a short/empty `query` with
	// success:false ("search value is empty"), which would look like a bad token.
	// advanced_search with empty parameters returns success when the session is
	// valid, so it's a clean auth probe.
	_, err := p.call(ctx, c, cfg, s, advancedSearchPath(1, "favorite", ""), "parameters="+url.QueryEscape("{}"))
	if err == nil {
		return nil
	}
	if nr, ok := source.AsNeedsRefresh(err); ok {
		return &source.ErrProviderVerify{Reason: verifyReason(nr.Layer)}
	}
	if errors.Is(err, errUnauth) {
		return &source.ErrProviderVerify{Reason: "invalid_token"}
	}
	return &source.ErrProviderVerify{Reason: "unreachable"}
}

func (p thirtynama) Search(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, q source.SearchQuery) (source.SearchResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	query := strings.TrimSpace(q.Query)
	var path, body string
	// full_search needs a term of at least 2 characters; the provider rejects a
	// shorter one as "empty". For an empty or single-char query we browse via
	// advanced_search instead (which also carries the filters).
	useFull := len([]rune(query)) >= 2
	if useFull {
		// full_search ONLY accepts type/all — a numeric code (type/15) or the slug
		// (type/movie) is answered with success:true and zero posts. So we always
		// browse all types here and narrow to the selected type client-side, below,
		// by each result's title_type. (Verified against the live API, spec 2002.)
		path = fmt.Sprintf("/api/v1/action/full_search/type/all/orderby/relevant/order/desc/page/%d", page)
		body = "query=" + url.QueryEscape(query)
	} else {
		path = advancedSearchPath(page, q.Sort, q.Order)
		body = "parameters=" + url.QueryEscape(buildParams(q.Filters))
	}
	raw, err := p.call(ctx, c, cfg, s, path, body)
	if err != nil {
		return source.SearchResult{}, runtimeErr(err)
	}
	// The two endpoints nest results differently: advanced_search returns
	// {page,pages,posts}; full_search wraps the title results one level deeper
	// under {title:{page,pages,posts}, person, news}.
	var r tnSearchResult
	if useFull {
		var fr struct {
			Title tnSearchResult `json:"title"`
		}
		if err := json.Unmarshal(raw, &fr); err != nil {
			return source.SearchResult{}, runtimeErr(errBadShape)
		}
		r = fr.Title
	} else if err := json.Unmarshal(raw, &r); err != nil {
		return source.SearchResult{}, runtimeErr(errBadShape)
	}
	out := source.SearchResult{Page: int(r.Page), Pages: int(r.Pages)}
	for _, post := range r.Posts {
		out.Items = append(out.Items, source.CatalogTitle{
			ID:                string(post.ID),
			Type:              string(post.TitleType),
			Title:             html.UnescapeString(string(post.Title)),
			PosterURL:         post.posterURL(),
			PosterFallbackURL: post.posterFallbackURL(),
			BackdropURL:       post.backdropURL(),
			IMDbID:            string(post.IMDbID),
			IMDbScore:         float64(post.IMDbScore),
			ProviderScore:     float64(post.Score),
			Plot:              html.UnescapeString(string(post.EnglishPlot)),
			Genres:            post.genreNames(),
			ComingSoon:        bool(post.ComingSoon),
			FreeDownload:      bool(post.FreeDownload),
		})
	}
	// full_search can't filter by type server-side (type/all only), so honor the
	// Type filter here by dropping results whose title_type doesn't match. Browse
	// (advanced_search) already filters server-side, so this applies only to the
	// text-search path.
	if useFull {
		if want := titleTypeForType(q.Filters.Type); want != "" {
			kept := out.Items[:0]
			for _, it := range out.Items {
				if it.Type == want {
					kept = append(kept, it)
				}
			}
			out.Items = kept
		}
	}
	return out, nil
}

// Parameters fetches the provider's advanced-search facet lists so the filter UI
// reflects the live source. Options whose value is empty (the provider's "All"
// entry) are dropped — the client supplies its own "Any".
func (p thirtynama) Parameters(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) (source.SearchParameters, error) {
	raw, err := p.call(ctx, c, cfg, s, "/api/v1/action/advanced_search_parametres", "")
	if err != nil {
		return source.SearchParameters{}, runtimeErr(err)
	}
	var r tnParams
	if err := json.Unmarshal(raw, &r); err != nil {
		return source.SearchParameters{}, runtimeErr(errBadShape)
	}
	return source.SearchParameters{
		Genres:    facetOptions(r.Genre),
		Types:     facetOptions(r.Type),
		Qualities: facetOptions(r.Quality),
		Scores:    facetOptions(r.Score),
		Languages: facetOptions(r.Language),
		Countries: facetOptions(r.Country),
		Channels:  facetOptions(r.Channel),
		Encoders:  stringFacets(r.Encoder),
		Ages:      stringFacets(r.Age),
		MinYear:   int(r.MinYear),
		MaxYear:   int(r.MaxYear),
	}, nil
}

// stringFacets turns a plain string list (encoder, age) into options, dropping
// blanks. The value doubles as the name — the client localizes if it can.
func stringFacets(in []flexStr) []source.FacetOption {
	out := make([]source.FacetOption, 0, len(in))
	for _, s := range in {
		v := string(s)
		if strings.TrimSpace(v) == "" {
			continue
		}
		out = append(out, source.FacetOption{Value: v, Name: v})
	}
	return out
}

// facetOptions converts the provider's facet entries to our shape, dropping the
// empty "All" entry and any entry missing a value.
func facetOptions(in []tnFacet) []source.FacetOption {
	out := make([]source.FacetOption, 0, len(in))
	for _, f := range in {
		v := string(f.Value)
		if v == "" {
			continue
		}
		out = append(out, source.FacetOption{Value: v, Name: html.UnescapeString(string(f.Name)), Slug: string(f.Slug)})
	}
	return out
}

func (p thirtynama) Title(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, id string) (source.TitleDetail, error) {
	quals, isSeries, err := p.downloads(ctx, c, cfg, s, id)
	if err != nil {
		return source.TitleDetail{}, runtimeErr(err)
	}
	typ := source.TypeMovie
	if isSeries {
		typ = source.TypeSeries
	}
	// Sendable when it exposes downloadable entries: a movie's files, or a
	// series' season packs.
	return source.TitleDetail{ID: id, Type: typ, Sendable: len(quals) > 0, Qualities: quals}, nil
}

func (p thirtynama) ResolveDownload(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, titleID, qualityID string) ([]string, string, error) {
	raw, err := p.call(ctx, c, cfg, s, "/api/v1/action/download/id/"+url.PathEscape(titleID), "")
	if err != nil {
		return nil, "", runtimeErr(err)
	}
	var r tnDownloadResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, "", runtimeErr(errBadShape)
	}
	for _, d := range r.Download {
		if d.ID == qualityID {
			links := entryLinks(d)
			if len(links) == 0 {
				return nil, "", errors.New("thirtynama: no download url for quality")
			}
			// Every signed link must live on an allowed download host.
			for _, link := range links {
				u, err := url.Parse(link)
				if err != nil || !source.HostAllowed(u.Hostname(), cfg.DownloadHosts) {
					return nil, "", source.ErrHostNotAllowed
				}
			}
			return links, d.Size, nil
		}
	}
	return nil, "", errors.New("thirtynama: quality not found")
}

// downloads fetches a title's downloadable entries and reports whether it is a
// series (its entries are season packs).
func (p thirtynama) downloads(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, id string) ([]source.QualityOption, bool, error) {
	raw, err := p.call(ctx, c, cfg, s, "/api/v1/action/download/id/"+url.PathEscape(id), "")
	if err != nil {
		return nil, false, err
	}
	var r tnDownloadResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, false, errBadShape
	}
	isSeries := bool(r.IsSeries)
	out := make([]source.QualityOption, 0, len(r.Download))
	for _, d := range r.Download {
		if len(entryLinks(d)) == 0 {
			continue // not downloadable (e.g. stream-only)
		}
		q := source.QualityOption{
			ID:         d.ID,
			Label:      html.UnescapeString(d.Quality),
			Size:       d.Size,
			Resolution: d.Resolution,
			Encoder:    html.UnescapeString(d.Encoder),
			Hardsub:    bool(d.Hardsub),
		}
		if isSeries {
			q.Season = seasonLabel(d)
			q.Episodes = int(d.TotalEpisode)
		}
		out = append(out, q)
	}
	return out, isSeries, nil
}

func seasonLabel(d tnDownload) string {
	if n := int(d.SeasonInt); n > 0 {
		return fmt.Sprintf("Season %d", n)
	}
	return html.UnescapeString(string(d.SeasonName))
}

// call performs an authenticated API POST and returns result raw JSON, mapping a
// non-JSON / non-success / origin-404 response to errUnauth. A clearance
// challenge surfaces as *source.ErrNeedsRefresh from the client and is passed
// through unchanged.
func (p thirtynama) call(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path, body string) (json.RawMessage, error) {
	headers, cookies := p.auth(s)
	resp, err := c.Do(ctx, s, cfg.APIHosts, source.Req{
		Method:  "POST",
		URL:     apiBase + path,
		Body:    body,
		XHR:     true,
		Origin:  tnOrigin,
		Referer: tnReferer,
		Headers: headers,
		Cookies: cookies,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != 200 {
		return nil, errUnauth
	}
	var env tnEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(resp.Body), &env); err != nil {
		return nil, errUnauth
	}
	if !env.Success {
		return nil, errUnauth
	}
	return env.Result, nil
}

// runtimeErr converts an internal error into a client-facing runtime error:
// unauth → needs-refresh(token); challenge → passed through.
func runtimeErr(err error) error {
	if _, ok := source.AsNeedsRefresh(err); ok {
		return err
	}
	if errors.Is(err, errUnauth) {
		return &source.ErrNeedsRefresh{Layer: source.LayerToken}
	}
	return err
}

func verifyReason(layer string) string {
	switch layer {
	case source.LayerClearance:
		return "challenge"
	case source.LayerIP:
		return "ip_mismatch"
	default:
		return "invalid_token"
	}
}

// typeCodes maps our friendly title-type names to the provider's numeric type
// codes (from advanced_search_parametres). The API only filters correctly on
// these codes — the plain names silently return movies.
var typeCodes = map[string]string{
	source.TypeMovie:  "15",
	source.TypeSeries: "16",
	source.TypeAnime:  "17",
}

// allowedOrderby is the set of provider orderby fields we expose; anything else
// falls back to most-popular descending (the app default).
var allowedOrderby = map[string]bool{"year": true, "date": true, "favorite": true, "imdb": true}

func orderbyField(sort string) string {
	if allowedOrderby[sort] {
		return sort
	}
	// Default: most popular, descending — the same default the client uses
	// (DEFAULT_SORT in useSourceCatalog.ts), so the two can't drift. It used to
	// be the release-year sort, but that one leads with the provider's
	// broken-year rows, which is the worst thing to land on (spec 2007).
	return "favorite"
}

// orderDir clamps the direction to the provider's two values; anything but an
// explicit "asc" browses descending (the app default, e.g. most popular first).
func orderDir(order string) string {
	if order == "asc" {
		return "asc"
	}
	return "desc"
}

func advancedSearchPath(page int, sort, order string) string {
	return fmt.Sprintf("/api/v1/action/advanced_search/page/%d/orderby/%s/order/%s", page, orderbyField(sort), orderDir(order))
}

// titleTypeForType resolves a type filter — a friendly name ("movie") or the
// provider's numeric code ("15") — to the title_type string the provider stamps
// on each result. Used to narrow full_search results client-side, since
// full_search only accepts type/all and cannot filter by type server-side.
// An empty filter (no selection) returns empty, meaning "keep all types".
func titleTypeForType(t string) string {
	if t == "" {
		return ""
	}
	// typeCodes' friendly names (movie/series/anime) ARE the title_type strings.
	for name, code := range typeCodes {
		if t == name || t == code {
			return name
		}
	}
	return t
}

// typeParam resolves a type filter to the provider's code: our friendly names
// (movie/series/anime) map through typeCodes; a value that's already a code (from
// the live facet list, e.g. "15") passes through. Empty stays empty.
func typeParam(t string) string {
	if t == "" {
		return ""
	}
	if code, ok := typeCodes[t]; ok {
		return code
	}
	return t
}

// buildParams maps our filters onto the provider's advanced-search parameters.
//
// It deliberately sends NO implicit year bounds. Spec 2006 added `min_year` /
// `max_year` to the release-year sort to hide the provider's ~350 broken-year
// rows (empty ones lead ascending; nonsense ones like "Reptile Royalty 7441"
// lead descending). The ordering that produced was correct, but timing the live
// API showed the `min_year` filter is what makes their query slow: 15–20s on any
// page they haven't cached, against 1.5–2s unbounded, while the other sorts run
// 0.4–3.9s. A lower bound (`min_year=1`) is no cheaper, so the cost is the
// filter itself, not the value. With two pages fetched per scroll trigger that
// landed twice per trigger, so spec 2007 traded the tidy head back for speed.
func buildParams(f source.SearchFilters) string {
	m := map[string]any{}
	if t := typeParam(f.Type); t != "" {
		m["type"] = t
	}
	if f.Quality != "" {
		m["quality"] = f.Quality
	}
	if len(f.Genre) > 0 {
		m["genre"] = f.Genre
	}
	if f.Language != "" {
		m["language"] = f.Language
	}
	if f.Country != "" {
		m["country"] = f.Country
	}
	if f.Score != "" {
		m["score"] = f.Score
	}
	if f.Age != "" {
		m["age"] = f.Age
	}
	if f.Channel != "" {
		m["channel"] = f.Channel
	}
	if f.Encoder != "" {
		m["encoder"] = f.Encoder
	}
	if f.X265 != "" {
		m["x265"] = f.X265
	}
	if f.ThreeD != "" {
		m["3d"] = f.ThreeD
	}
	if f.Cast != "" {
		m["cast"] = f.Cast
	}
	if f.Director != "" {
		m["director"] = f.Director
	}
	if f.Creator != "" {
		m["creator"] = f.Creator
	}
	if f.YearFrom != "" {
		m["min_year"] = f.YearFrom
	}
	if f.YearTo != "" {
		m["max_year"] = f.YearTo
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// --- upstream JSON shapes (only the fields we use) ---

type tnEnvelope struct {
	Success bool            `json:"success"`
	Msg     *string         `json:"msg"`
	Result  json.RawMessage `json:"result"`
}

type tnSearchResult struct {
	Page  flexInt  `json:"page"`
	Pages flexInt  `json:"pages"`
	Posts []tnPost `json:"posts"`
}

// The provider is loosely typed: empty string fields come back as `false`
// (a bool), and ids may be numbers or strings. Every field that isn't strictly
// typed uses a flex* type so one coverless/oddly-typed post never fails the
// whole page's parse.
type tnPost struct {
	ID           flexNumStr `json:"id"`
	TitleType    flexStr    `json:"title_type"`
	Title        flexStr    `json:"title"`
	IMDbID       flexStr    `json:"imdb_id"`
	IMDbScore    flexFloat  `json:"imdb_score"`
	Score        flexFloat  `json:"30nama_score"`
	ComingSoon   flexBool   `json:"coming_soon"`
	FreeDownload flexBool   `json:"free_download"`
	EnglishPlot  flexStr    `json:"english_plot"`
	Image        struct {
		Cover  flexStr `json:"cover"`
		Poster struct {
			// Sized portrait posters. Coming-soon titles have an empty `cover`
			// but still carry the provider's default placeholder here (the
			// "none-*_30NAMA" images), so it doubles as the missing-cover fallback.
			Big    flexStr `json:"big"`
			Large  flexStr `json:"large"`
			Medium flexStr `json:"medium"`
			Small  flexStr `json:"small"`
		} `json:"poster"`
	} `json:"image"`
	Genre []struct {
		Name flexStr `json:"name"`
		Slug flexStr `json:"slug"`
	} `json:"genre"`
}

// posterURL picks the best available cover art: the landscape `cover` when the
// title has real art, otherwise the sized portrait poster (which for coming-soon
// titles is the provider's default placeholder rather than nothing).
// bestPoster returns the largest-available sized portrait poster (or "").
func (p tnPost) bestPoster() string {
	for _, s := range []string{
		string(p.Image.Poster.Medium),
		string(p.Image.Poster.Large),
		string(p.Image.Poster.Big),
		string(p.Image.Poster.Small),
	} {
		if s != "" {
			return s
		}
	}
	return ""
}

// isPlaceholderImg reports whether a provider image URL is the generic
// "no artwork yet" placeholder (used for coming-soon titles without a poster).
func isPlaceholderImg(u string) bool { return strings.Contains(u, "/none/none-") }

// posterURL is the PORTRAIT poster for grids and the detail thumbnail. The
// provider's sized `poster.*` is a consistent portrait image, so prefer it; the
// `cover` is only used when there's no real poster (it is a portrait poster for
// some titles but a landscape backdrop for others, so it's the wrong shape for a
// thumbnail). A bare placeholder yields to a real cover when one exists.
func (p tnPost) posterURL() string {
	poster := p.bestPoster()
	if poster != "" && !isPlaceholderImg(poster) {
		return poster
	}
	if c := string(p.Image.Cover); c != "" {
		return c
	}
	return poster // the placeholder (coming-soon with no cover) — better than nothing
}

// backdropURL is the wide `cover` image, shown large behind the detail header.
// Empty when the title has no cover (then the header falls back to the poster).
func (p tnPost) backdropURL() string {
	if c := string(p.Image.Cover); c != "" && c != p.posterURL() {
		return c
	}
	return ""
}

// posterFallbackURL is the reliable sized portrait poster (or the provider's
// placeholder for titles without art), always on the provider CDN. The client
// uses it when the primary posterURL fails to load — e.g. a title whose `cover`
// is a real-looking URL that actually 404s, which would otherwise show nothing.
// posterFallbackURL is the OTHER available image, tried by the client if the
// primary poster fails to load (a present-but-404 URL).
func (p tnPost) posterFallbackURL() string {
	primary := p.posterURL()
	if c := string(p.Image.Cover); c != "" && c != primary {
		return c
	}
	if ps := p.bestPoster(); ps != "" && ps != primary {
		return ps
	}
	return ""
}

// genreNames returns human genre labels (slug-derived), skipping empties.
func (p tnPost) genreNames() []string {
	out := make([]string, 0, len(p.Genre))
	for _, g := range p.Genre {
		if s := string(g.Slug); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// tnParams is the advanced_search_parametres result. Facet values are loosely
// typed (numbers, strings, or a compound like "17&124913"), so flexNumStr keeps
// each as its textual form.
type tnParams struct {
	Genre    []tnFacet `json:"genre"`
	Type     []tnFacet `json:"type"`
	Quality  []tnFacet `json:"quality"`
	Score    []tnFacet `json:"score"`
	Country  []tnFacet `json:"country"`
	Language []tnFacet `json:"language"`
	Channel  []tnFacet `json:"channel"`
	// encoder and age are plain string arrays, not {name,value} objects.
	Encoder []flexStr `json:"encoder"`
	Age     []flexStr `json:"age"`
	MinYear flexInt   `json:"min_year"`
	MaxYear flexInt   `json:"max_year"`
}

type tnFacet struct {
	Name  flexStr    `json:"name"`
	Slug  flexStr    `json:"slug"`
	Value flexNumStr `json:"value"`
}

type tnDownloadResult struct {
	IsSeries flexBool     `json:"is_series"`
	Download []tnDownload `json:"download"`
}

type tnDownload struct {
	ID         string   `json:"id"`
	Quality    string   `json:"quality"`
	Size       string   `json:"size"`
	Resolution string   `json:"resolution"`
	Encoder    string   `json:"encoder"`
	Hardsub    flexBool `json:"hardsub"`
	// A movie exposes its file directly on dl/ipdl. A series season pack instead
	// carries one link entry PER EPISODE under `link`, plus season info.
	DL           string   `json:"dl"`
	IPDL         string   `json:"ipdl"`
	Link         []tnLink `json:"link"`
	SeasonName   flexStr  `json:"season_name"`
	SeasonInt    flexInt  `json:"season_int"`
	TotalEpisode flexInt  `json:"total_episode"`
}

type tnLink struct {
	ID   string `json:"id"`
	DL   string `json:"dl"`
	IPDL string `json:"ipdl"`
}

// entryLinks returns every signed URL for a download entry: one for a movie, one
// per episode for a series season pack.
func entryLinks(d tnDownload) []string {
	if d.DL != "" {
		return []string{d.DL}
	}
	if d.IPDL != "" {
		return []string{d.IPDL}
	}
	out := make([]string, 0, len(d.Link))
	for _, e := range d.Link {
		if e.DL != "" {
			out = append(out, e.DL)
		} else if e.IPDL != "" {
			out = append(out, e.IPDL)
		}
	}
	return out
}

// flexFloat/flexInt/flexBool tolerate numbers, strings, or null (the provider is
// loosely typed across fields).

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil // tolerate non-numeric strings
		}
		*f = flexFloat(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return nil // non-numeric (e.g. the provider's `false` for empty) → 0
	}
	*f = flexFloat(v)
	return nil
}

type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var ff flexFloat
	if err := ff.UnmarshalJSON(b); err != nil {
		return err
	}
	*f = flexInt(int(ff))
	return nil
}

type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch string(b) {
	case "", "null", "false", "0", `"0"`, `""`:
		*f = false
	case "true", "1", `"1"`:
		*f = true
	default:
		var v bool
		if err := json.Unmarshal(b, &v); err == nil {
			*f = flexBool(v)
		}
	}
	return nil
}

// flexStr accepts a JSON string and yields it; anything else the provider sends
// for an "empty" value (bool false, null, number, object) yields "". This is the
// key robustness against the provider returning `false` for a missing poster.
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexStr(s)
	}
	return nil
}

// flexNumStr accepts an id that may be a JSON number or a string, yielding its
// textual form ("" for null).
type flexNumStr string

func (f *flexNumStr) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexNumStr(s)
		return nil
	}
	*f = flexNumStr(string(b)) // a JSON number → its literal digits
	return nil
}

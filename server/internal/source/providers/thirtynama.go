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

// Hosts is the provider's fixed outbound allowlist: the API host plus the site
// (for clearance verification) and the signed-download storage domain. The
// download storage host rotates its subdomain (eu-download-storage-NN.*), so it
// is allowed by domain suffix.
func (thirtynama) Hosts() source.Config {
	return source.Config{
		APIHosts:      []string{tnAPIHost, "30nama.com"},
		DownloadHosts: []string{"divyacamilla.info"},
	}
}

func (p thirtynama) VerifySession(ctx context.Context, c *source.Client, cfg source.Config, s source.Session) error {
	// Cheapest authenticated call: an empty advanced_search (parameters={}).
	// NOT full_search — the provider rejects a short/empty `query` with
	// success:false ("search value is empty"), which would look like a bad token.
	// advanced_search with empty parameters returns success when the session is
	// valid, so it's a clean auth probe.
	_, err := p.call(ctx, c, cfg, s, advancedSearchPath(1, "favorite"), "parameters="+url.QueryEscape("{}"))
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
		path = fmt.Sprintf("/api/v1/action/full_search/type/all/orderby/relevant/order/desc/page/%d", page)
		body = "query=" + url.QueryEscape(query)
	} else {
		path = advancedSearchPath(page, q.Sort)
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
			ID:            string(post.ID),
			Type:          string(post.TitleType),
			Title:         string(post.Title),
			PosterURL:     string(post.Image.Cover),
			IMDbID:        string(post.IMDbID),
			IMDbScore:     float64(post.IMDbScore),
			ProviderScore: float64(post.Score),
			ComingSoon:    bool(post.ComingSoon),
			FreeDownload:  bool(post.FreeDownload),
		})
	}
	return out, nil
}

func (p thirtynama) Title(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, id string) (source.TitleDetail, error) {
	quals, err := p.downloads(ctx, c, cfg, s, id)
	if err != nil {
		return source.TitleDetail{}, runtimeErr(err)
	}
	// v1: a title is sendable when it exposes movie-style download entries.
	return source.TitleDetail{ID: id, Type: source.TypeMovie, Sendable: len(quals) > 0, Qualities: quals}, nil
}

func (p thirtynama) ResolveDownload(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, titleID, qualityID string) (string, string, error) {
	raw, err := p.call(ctx, c, cfg, s, "/api/v1/action/download/id/"+url.PathEscape(titleID), "")
	if err != nil {
		return "", "", runtimeErr(err)
	}
	var r tnDownloadResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", "", runtimeErr(errBadShape)
	}
	for _, d := range r.Download {
		if d.ID == qualityID {
			link := d.DL
			if link == "" {
				link = d.IPDL
			}
			if link == "" {
				return "", "", errors.New("thirtynama: no download url for quality")
			}
			// The signed link must live on an allowed download host.
			u, err := url.Parse(link)
			if err != nil || !source.HostAllowed(u.Hostname(), cfg.DownloadHosts) {
				return "", "", source.ErrHostNotAllowed
			}
			return link, d.Size, nil
		}
	}
	return "", "", errors.New("thirtynama: quality not found")
}

// downloads fetches and maps a title's quality entries (without exposing links).
func (p thirtynama) downloads(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, id string) ([]source.QualityOption, error) {
	raw, err := p.call(ctx, c, cfg, s, "/api/v1/action/download/id/"+url.PathEscape(id), "")
	if err != nil {
		return nil, err
	}
	var r tnDownloadResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, errBadShape // odd shape (e.g. a series) — not an auth failure
	}
	out := make([]source.QualityOption, 0, len(r.Download))
	for _, d := range r.Download {
		if d.DL == "" && d.IPDL == "" {
			continue // not downloadable (e.g. stream-only)
		}
		out = append(out, source.QualityOption{
			ID:         d.ID,
			Label:      d.Quality,
			Size:       d.Size,
			Resolution: d.Resolution,
			Encoder:    d.Encoder,
			Hardsub:    bool(d.Hardsub),
		})
	}
	return out, nil
}

// call performs an authenticated API POST and returns result raw JSON, mapping a
// non-JSON / non-success / origin-404 response to errUnauth. A clearance
// challenge surfaces as *source.ErrNeedsRefresh from the client and is passed
// through unchanged.
func (p thirtynama) call(ctx context.Context, c *source.Client, cfg source.Config, s source.Session, path, body string) (json.RawMessage, error) {
	resp, err := c.Do(ctx, s, cfg.APIHosts, source.Req{
		Method:  "POST",
		URL:     apiBase + path,
		Body:    body,
		XHR:     true,
		Origin:  tnOrigin,
		Referer: tnReferer,
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
// falls back to release-year descending (the app default).
var allowedOrderby = map[string]bool{"year": true, "date": true, "favorite": true}

func orderbyField(sort string) string {
	if allowedOrderby[sort] {
		return sort
	}
	return "year" // default: release year, descending
}

func advancedSearchPath(page int, sort string) string {
	return fmt.Sprintf("/api/v1/action/advanced_search/page/%d/orderby/%s/order/desc", page, orderbyField(sort))
}

func buildParams(f source.SearchFilters) string {
	m := map[string]any{}
	if f.Type != "" {
		if code, ok := typeCodes[f.Type]; ok {
			m["type"] = code
		} else {
			m["type"] = f.Type
		}
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
	Image        struct {
		Cover flexStr `json:"cover"`
	} `json:"image"`
}

type tnDownloadResult struct {
	Download []tnDownload `json:"download"`
}

type tnDownload struct {
	ID         string   `json:"id"`
	Quality    string   `json:"quality"`
	Size       string   `json:"size"`
	Resolution string   `json:"resolution"`
	Encoder    string   `json:"encoder"`
	Hardsub    flexBool `json:"hardsub"`
	DL         string   `json:"dl"`
	IPDL       string   `json:"ipdl"`
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

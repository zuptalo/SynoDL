// Package source is SynoDL's generic download-source abstraction (spec 0005).
//
// It lets an operator-configured external "provider" be browsed/searched and its
// items sent to Download Station, WITHOUT hardcoding one site into the core:
//   - a Provider driver maps generic operations onto one site's API, and
//   - a shared stdlib HTTP/2 Client carries the admin's session material and
//     refuses any host outside the provider's configured allowlist.
//
// This is the second outbound target in SynoDL (beyond the NAS); it is
// operator-opt-in, off by default, and bounded to the configured hosts — see the
// constitution (Principle III) and specs/0005-source-catalog.
package source

import (
	"context"
	"sort"
	"sync"
)

// TitleType values.
const (
	TypeMovie  = "movie"
	TypeSeries = "series"
	TypeAnime  = "anime"
)

// Session is the (secret) material the server sends to the provider. It mirrors
// store.SourceSession but is defined here so this package doesn't depend on the
// store. Never serialize it to a client.
type Session struct {
	CFClearance string
	CAPIKey     string
	CToken      string
	UserAgent   string
	CPlatform   string
	CAppVersion string
}

// Config is a provider's non-secret settings needed to make calls: the outbound
// host allowlists.
type Config struct {
	APIHosts      []string
	DownloadHosts []string
	ImageHosts    []string // poster/cover CDN hosts the image proxy may fetch
}

// ImageHostAllowed reports whether host is a poster/cover host for ANY registered
// provider. Used by the (unauthenticated) image proxy to stay bounded to known
// public CDN image hosts — never an open proxy.
func ImageHostAllowed(host string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, p := range registry {
		if HostAllowed(host, p.Hosts().ImageHosts) {
			return true
		}
	}
	return false
}

// SearchFilters mirrors the provider's advanced-search facets. Empty fields are
// omitted.
type SearchFilters struct {
	Type     string
	Quality  string
	Language string
	Country  string
	Score    string
	Age      string // content rating (e.g. "G"); forced server-side for capped users
	Genre    []string
	// Additional advanced-search facets (spec 1013).
	Channel  string // TV channel/network (e.g. "Netflix")
	Encoder  string // release group (e.g. "YIFY")
	X265     string // "true" to require an x265/HEVC encode
	ThreeD   string // "true" to require a 3D release
	Stream   string // "true" to require a streamable copy
	Cast     string // free-text actor name
	Director string // free-text director name
	Creator  string // free-text creator name
	YearFrom string // release-year lower bound
	YearTo   string // release-year upper bound
}

// FacetOption is one selectable value in a filter facet. Name is the provider's
// own label (may be non-English); Slug is its English slug when the provider
// gives one (genres do). The client resolves the display label from Value/Slug.
type FacetOption struct {
	Value string `json:"value"`
	Name  string `json:"name,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

// SearchParameters is the provider's current set of filter facets — what the
// advanced-search UI should offer. Empty slices are fine (the client falls back
// to its built-in list).
type SearchParameters struct {
	Genres    []FacetOption `json:"genres"`
	Types     []FacetOption `json:"types"`
	Qualities []FacetOption `json:"qualities"`
	Scores    []FacetOption `json:"scores"`
	Languages []FacetOption `json:"languages"`
	Countries []FacetOption `json:"countries"`
	Channels  []FacetOption `json:"channels"`
	Encoders  []FacetOption `json:"encoders"`
	Ages      []FacetOption `json:"ages"`
	MinYear   int           `json:"minYear"`
	MaxYear   int           `json:"maxYear"`
}

// SearchQuery is one catalog query.
type SearchQuery struct {
	Query   string
	Page    int
	Sort    string // provider orderby field for browse (e.g. "year"); "" = default
	Order   string // "asc" / "desc"; "" = provider default (descending)
	Filters SearchFilters
}

// CatalogTitle is one search result (no download links).
type CatalogTitle struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	PosterURL string `json:"posterUrl"`
	// A reliable secondary poster (the provider's sized/placeholder image) the
	// client falls back to when PosterURL fails to load — e.g. a title whose
	// primary cover URL is present but 404s. Empty when there is no alternative.
	PosterFallbackURL string `json:"posterFallbackUrl,omitempty"`
	// The wide "cover" image, shown large behind the detail header. Empty when the
	// title has no distinct backdrop (then the header uses the poster).
	BackdropURL   string   `json:"backdropUrl,omitempty"`
	IMDbID        string   `json:"imdbId"`
	IMDbScore     float64  `json:"imdbScore"`
	ProviderScore float64  `json:"providerScore"`
	Plot          string   `json:"plot"`
	Genres        []string `json:"genres"`
	ComingSoon    bool     `json:"comingSoon"`
	FreeDownload  bool     `json:"freeDownload"`
}

// SearchResult is a page of results.
type SearchResult struct {
	Page  int            `json:"page"`
	Pages int            `json:"pages"`
	Items []CatalogTitle `json:"items"`
}

// QualityOption is a downloadable variant of a title. The signed URL it resolves
// to is NEVER included here — it is fetched at send time (see ResolveDownload).
// For a series each option is a season pack, so Season/Episodes are set.
type QualityOption struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Size       string `json:"size"`
	Resolution string `json:"resolution"`
	Encoder    string `json:"encoder"`
	Hardsub    bool   `json:"hardsub"`
	Season     string `json:"season,omitempty"`
	Episodes   int    `json:"episodes,omitempty"`
}

// TitleDetail is a title with its qualities. Sendable is false for types this
// provider/version cannot send (v1: series/anime).
type TitleDetail struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Sendable  bool            `json:"sendable"`
	Qualities []QualityOption `json:"qualities"`
}

// Provider maps generic catalog operations onto a specific site. Implementations
// are stateless: all secrets/config arrive per call so the same driver serves
// any configured instance.
type Provider interface {
	// Kind is the stable registry key (e.g. "thirtynama").
	Kind() string
	// Hosts returns the provider's fixed outbound allowlist (API + signed
	// download hosts). The admin never types these; they are provider-defined so
	// the outbound surface stays bounded and known.
	Hosts() Config
	// VerifySession performs a cheap authenticated call; nil means the session
	// works. On failure it returns *ErrProviderVerify or *ErrNeedsRefresh.
	VerifySession(ctx context.Context, c *Client, cfg Config, s Session) error
	// Search returns a page of catalog results for the query/filters.
	Search(ctx context.Context, c *Client, cfg Config, s Session, q SearchQuery) (SearchResult, error)
	// Parameters returns the provider's current facet options (genres, types,
	// qualities, languages, countries, score bands, year bounds) so the filter UI
	// stays in step with the source instead of a hardcoded list.
	Parameters(ctx context.Context, c *Client, cfg Config, s Session) (SearchParameters, error)
	// Title returns a title's detail + qualities (movies are Sendable).
	Title(ctx context.Context, c *Client, cfg Config, s Session, id string) (TitleDetail, error)
	// ResolveDownload re-fetches and returns the signed URL(s) and the human size
	// for one quality of a title, at send time (links are never cached): one URL
	// for a movie, one per episode for a series season pack. Every returned host
	// must be in cfg.DownloadHosts.
	ResolveDownload(ctx context.Context, c *Client, cfg Config, s Session, titleID, qualityID string) (links []string, size string, err error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Provider{}
)

// Register adds a provider driver by kind. Called from driver init().
func Register(p Provider) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Kind()] = p
}

// Get returns the driver for kind (nil, false when unknown).
func Get(kind string) (Provider, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := registry[kind]
	return p, ok
}

// Kinds lists registered provider kinds (sorted, for admin UI).
func Kinds() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

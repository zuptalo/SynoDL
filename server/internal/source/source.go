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
}

// SearchFilters mirrors the provider's advanced-search facets. Empty fields are
// omitted.
type SearchFilters struct {
	Type     string
	Quality  string
	Language string
	Country  string
	Score    string
	Age      string // content rating (e.g. "G"); set server-side for capped users
	Genre    []string
}

// SearchQuery is one catalog query.
type SearchQuery struct {
	Query   string
	Page    int
	Sort    string // provider orderby field for browse (e.g. "year"); "" = default
	Filters SearchFilters
}

// CatalogTitle is one search result (no download links).
type CatalogTitle struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	PosterURL     string  `json:"posterUrl"`
	IMDbID        string  `json:"imdbId"`
	IMDbScore     float64 `json:"imdbScore"`
	ProviderScore float64 `json:"providerScore"`
	ComingSoon    bool    `json:"comingSoon"`
	FreeDownload  bool    `json:"freeDownload"`
}

// SearchResult is a page of results.
type SearchResult struct {
	Page  int            `json:"page"`
	Pages int            `json:"pages"`
	Items []CatalogTitle `json:"items"`
}

// QualityOption is a downloadable variant of a title. The signed URL it resolves
// to is NEVER included here — it is fetched at send time (see ResolveDownload).
type QualityOption struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Size       string `json:"size"`
	Resolution string `json:"resolution"`
	Encoder    string `json:"encoder"`
	Hardsub    bool   `json:"hardsub"`
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
	// Title returns a title's detail + qualities (movies are Sendable).
	Title(ctx context.Context, c *Client, cfg Config, s Session, id string) (TitleDetail, error)
	// ResolveDownload re-fetches and returns the signed URL for one quality of a
	// title, at send time (links are never cached). The returned host must be in
	// cfg.DownloadHosts.
	ResolveDownload(ctx context.Context, c *Client, cfg Config, s Session, titleID, qualityID string) (string, error)
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

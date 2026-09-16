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
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Degradation / verify reason categories. Kept here so the API, the drivers and
// the store all name the same states.
const (
	ReasonNeedsRefresh = "needs_refresh"
	ReasonUnsubscribed = "unsubscribed"
	ReasonUnreachable  = "unreachable"
	ReasonTimeout      = "timeout"
	// ReasonFilterUnsupported: the user narrowed by something this source has no
	// equivalent for. It is skipped rather than queried without the narrowing,
	// because unfiltered results shown alongside filtered ones read as the filter
	// being broken (spec 1024, FR-007).
	ReasonFilterUnsupported = "filter_unsupported"
)

// TitleType values.
const (
	TypeMovie  = "movie"
	TypeSeries = "series"
	TypeAnime  = "anime"
)

// Session is the (secret) material the server sends to ONE provider. It mirrors
// store.SourceSession but is defined here so this package doesn't depend on the
// store. Never serialize it to a client.
//
// Fields is a provider-declared bag rather than a fixed struct. That is
// deliberate, and it is a containment measure as much as a generalization: with
// two sources configured, a fixed struct meant every driver saw — and the shared
// client blindly sent — every other driver's credentials. Now a driver receives
// only the keys it declared in SessionFields, and builds its own headers and
// cookies from them (see Req.Headers / Req.Cookies).
type Session struct {
	Fields    map[string]string
	UserAgent string
}

// Get returns one declared field ("" when absent).
func (s Session) Get(key string) string { return s.Fields[key] }

// SessionField describes one piece of material a provider needs, so the admin UI
// can render the right form without knowing anything about the provider. Help is
// shown at the point of paste — which is where a provider whose material is
// unusually powerful (a full account cookie rather than a scoped token) must say
// so, per the constitution's credential-safety rules.
type SessionField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Help     string `json:"help,omitempty"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// Config is a provider's non-secret settings needed to make calls: the outbound
// host allowlists, plus the optional alternate base URL to fall back to.
type Config struct {
	APIHosts      []string
	DownloadHosts []string
	ImageHosts    []string // poster/cover CDN hosts the image proxy may fetch
	// AltBase is an operator-set mirror for THIS source, used when the main
	// domain is unavailable (spec 1020). Sites of this kind get blocked
	// periodically and publish an alternate address; without this, a routine
	// outage silently removes a source the operator is paying for.
	//
	// Operator-set, not discovered: a site must never be able to advertise its
	// own next mirror and have us adopt it, because that would let it redirect
	// our credentials at will.
	AltBase string
	// MainBase is an operator-set primary address, overriding the driver's own
	// built-in one. Empty keeps the built-in address (spec 0009).
	//
	// Neither address is privileged: whichever is answering is the one that
	// matters. This exists because a site of this kind changes address on its own
	// schedule, and waiting for a release to follow it is not a plan.
	MainBase string
	// AltSession is the material belonging to AltBase, when the operator gave that
	// address its own. nil means the source has one set for both, which is what
	// every source configured before this has.
	//
	// It exists because credentials do not travel between addresses: a challenge
	// cookie is issued per domain, and a login cookie is tied to the address that
	// issued it. Sending one address's material to the other is how an outage
	// turned into a catalog that answered every request with a login page.
	AltSession *Session
}

// SessionFor returns the material to use when calling base.
func (c Config) SessionFor(base string, s Session) Session {
	if c.AltSession == nil || c.AltBase == "" {
		return s
	}
	if strings.TrimRight(strings.TrimSpace(base), "/") == strings.TrimRight(strings.TrimSpace(c.AltBase), "/") {
		return *c.AltSession
	}
	return s
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

// reResolution matches the resolution tokens release names and quality labels use.
var reResolution = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|360p)\b`)

// ResolutionOf reads a resolution out of free text — a quality label, a release
// name — and normalises 4K/UHD onto the token everything else uses.
//
// Drivers should prefer whatever the source states explicitly; this is the
// fallback for a source that leaves the field empty but names the resolution in
// its label anyway, which one of them does for every option it returns. Without
// it those options carry no resolution at all, and anything comparing releases
// cannot see them (spec 2011).
func ResolutionOf(s string) string {
	if m := reResolution.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1])
	}
	if regexp.MustCompile(`(?i)\b(4k|uhd)\b`).MatchString(s) {
		return "2160p"
	}
	return ""
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
	// Sorts are the orderings this source can actually honour, declared like any
	// other facet so combined browsing can intersect them (spec 1024). Their
	// values are the CANONICAL sort keys the client speaks ("imdb", "year",
	// "date", "favorite"); each driver maps those onto whatever its own site
	// calls them. A source that cannot order at all leaves this empty, and the
	// sort control offers nothing rather than something that quietly does nothing.
	Sorts   []FacetOption `json:"sorts"`
	MinYear int           `json:"minYear"`
	MaxYear int           `json:"maxYear"`
}

// SearchQuery is one catalog query.
type SearchQuery struct {
	Query   string
	Page    int
	Sort    string // canonical sort key for browse (e.g. "year"); "" = default
	Order   string // "asc" / "desc"; "" = provider default (descending)
	Filters SearchFilters
	// PerSource carries each source's OWN spelling of the filters above, resolved
	// by the API layer from that source's declared facets (spec 1024).
	//
	// Two sources name the same genre differently — a numeric code on one, a
	// Persian word on the other — so one shared value cannot be sent to both. A
	// ref absent from this map is queried with Filters as-is, which is what the
	// single-source path does. A ref mapped to nil has NO equivalent for what the
	// user chose and is skipped entirely rather than queried unfiltered.
	PerSource map[int64]*SearchFilters
}

// CatalogTitle is one search result (no download links).
type CatalogTitle struct {
	ID string `json:"id"`
	// Which configured source produced this item. SourceName is shown as a label
	// in combined mode so a title carried by two sources reads as two sources
	// offering it, rather than as a duplicate (FR-005a / FR-012a).
	SourceID   int64  `json:"sourceId,omitempty"`
	SourceName string `json:"sourceName,omitempty"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	PosterURL  string `json:"posterUrl"`
	// A reliable secondary poster (the provider's sized/placeholder image) the
	// client falls back to when PosterURL fails to load — e.g. a title whose
	// primary cover URL is present but 404s. Empty when there is no alternative.
	PosterFallbackURL string `json:"posterFallbackUrl,omitempty"`
	// The wide "cover" image, shown large behind the detail header. Empty when the
	// title has no distinct backdrop (then the header uses the poster).
	BackdropURL string `json:"backdropUrl,omitempty"`
	IMDbID      string `json:"imdbId"`
	// Year is the release year as the SOURCE publishes it — a bare "1974" for a
	// film, and empty when the source has no such field (30nama has none; its
	// years live at the end of the title string, which the client splits off).
	//
	// A string rather than a number: a series carries a range, and an ongoing
	// one an open range, neither of which fits an int. It is also deliberately
	// NOT validated here — the driver reports what the source said, and the
	// client decides what is plausible enough to show (spec 1032, FR-006).
	Year          string   `json:"year,omitempty"`
	IMDbScore     float64  `json:"imdbScore"`
	ProviderScore float64  `json:"providerScore"`
	Plot          string   `json:"plot"`
	Genres        []string `json:"genres"`
	ComingSoon    bool     `json:"comingSoon"`
	FreeDownload  bool     `json:"freeDownload"`
	// Ownership reports what is actually on the NAS for this title — see the
	// Ownership* constants. Set by the handler on the way out, never by a driver.
	//
	// It replaced a boolean deliberately: a folder EXISTING was taken as proof of
	// ownership in 0.3.0, and a folder holding only season.nfo was marked owned.
	// (formerly reported as "InLibrary")
	//
	// The original note read: reports that a folder for this title already exists under the
	// configured parent on the NAS (spec 0008), so Discover can mark it and the
	// user does not download it twice.
	//
	// Set by the API layer from the library snapshot — drivers never populate it,
	// exactly like SourceID/SourceName above. Omitted when false, which keeps the
	// payload byte-identical for the majority of titles and means an absent field
	// and `false` are the same answer: "not present, or not known".
	Ownership string `json:"ownership,omitempty"`
}

// SearchResult is a page of results.
type SearchResult struct {
	Page  int            `json:"page"`
	Pages int            `json:"pages"`
	Items []CatalogTitle `json:"items"`
	// Sources that couldn't answer this query. A failing source never fails the
	// whole request — the healthy ones still render and this explains the gap
	// (FR-012). Empty for a single-source query that succeeded.
	Degraded []DegradedSource `json:"degraded,omitempty"`
}

// DegradedSource names a source that dropped out of a combined query. Reason is
// a category only — never an upstream body, URL, or anything secret-derived.
type DegradedSource struct {
	SourceID int64  `json:"sourceId"`
	Name     string `json:"name"`
	Reason   string `json:"reason"` // needs_refresh | unsubscribed | unreachable | timeout
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

	// Owned reports that THIS release is the one already on the NAS — not merely
	// that something for this season is. Set by the handler on the way out, never
	// by a driver, like Ownership and Seasons on TitleDetail.
	//
	// It is deliberately absent far more often than Ownership is: identifying a
	// release needs the files to name both a resolution and a release group, and
	// when they do not, no option is marked at all. Marking on the resolution
	// alone was the bug (spec 1025) — several releases of one season share a
	// resolution and differ only by who encoded them, so it marked three options
	// out of four wrongly.
	Owned bool `json:"owned,omitempty"`

	// ReleaseName is the file this option would produce, as its download link
	// names it. Set by the driver where it knows; empty where it does not.
	//
	// It is how ownership is decided wherever it is present, because it is the one
	// thing a source cannot overwrite: a source may rename the release tokens
	// inside a file name — one renames every file it serves with its own suffix,
	// collapsing four encoders onto one — but a given link still yields a given
	// file, and that file is what lands on the NAS (spec 1026).
	//
	// `json:"-"` is deliberate and load-bearing: this must never reach the client,
	// and a wire tag enforces that permanently rather than relying on every future
	// handler to remember (FR-002).
	ReleaseName string `json:"-"`
}

// Person is somebody who worked on a title — a cast member or a member of its
// crew (spec 0014). Everything but Name is optional, because every source is
// missing something: one publishes the character but often no photograph, the
// other publishes neither on the page that names them.
//
// A missing field is a fact, not a failure. An empty IMDbID means the tile is
// not a link; an empty PhotoURL means "ask the person-photo endpoint, or show
// initials" — never "something went wrong".
type Person struct {
	Name string `json:"name"`
	// Character is the part they play, where the source publishes one. Only one
	// of the two does.
	Character string `json:"character,omitempty"`
	// IMDbID is their public "nm…" identity. It is what the tile links to and
	// what the photo cache is keyed by, so two people who share a name never
	// share a face.
	IMDbID string `json:"imdbId,omitempty"`
	// PhotoURL is a photograph the SOURCE hosts, already known to be a real one:
	// a provider's own "no photo" stand-in is dropped by the driver rather than
	// forwarded (FR-013), because a grey silhouette shown as somebody's face is
	// worse than no face at all.
	PhotoURL string `json:"photoUrl,omitempty"`
	// Ref is the source's own handle for this person ("actor/kurt-russell"), used
	// only by a source that names people without identifying them, so the server
	// can resolve them once and remember it.
	//
	// `json:"-"` is deliberate and load-bearing, exactly as it is on
	// QualityOption.ReleaseName: this is an internal join key, and a wire tag
	// enforces that permanently rather than relying on every future handler to
	// remember.
	Ref string `json:"-"`
}

// Field bounds for anything a source publishes about a person (FR-030a).
//
// A name, a character and a URL all arrive from outside. Without a bound, a
// hostile or merely broken upstream could put a megabyte into the operator's
// volume and into every viewer's DOM. These are generous — the longest real
// name is a fraction of them — so they clip an attack, never a person.
const (
	maxPersonName = 200
	maxPersonURL  = 2048
)

// Clamp returns p with every externally-supplied field bounded, and reports
// whether it is worth showing at all (a person with no name is not).
func (p Person) Clamp() (Person, bool) {
	p.Name = clampText(p.Name, maxPersonName)
	p.Character = clampText(p.Character, maxPersonName)
	p.PhotoURL = clampText(p.PhotoURL, maxPersonURL)
	p.Ref = clampText(p.Ref, maxPersonURL)
	if !PersonIDRe.MatchString(p.IMDbID) {
		// Anything that is not an IMDb person id is dropped rather than carried:
		// it is about to become an href and a cache key, and neither tolerates a
		// value we cannot vouch for.
		p.IMDbID = ""
	}
	return p, p.Name != ""
}

// clampText trims and bounds one free-text field, cutting on a rune boundary so
// a clipped name is never invalid UTF-8.
func clampText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	for len(string(r)) > max {
		r = r[:len(r)-1]
	}
	return string(r)
}

// PersonIDRe is a canonical IMDb person id. Anchored at both ends: this value
// reaches an href, a cache key and a URL path, and a loose match would let a
// source steer all three.
var PersonIDRe = regexp.MustCompile(`^nm\d{6,9}$`)

// PersonID normalises an IMDb person id out of either shape a source publishes —
// a bare "nm0000621" or the full profile URL one of them links to — and returns
// "" for anything else.
func PersonID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if m := rePersonURL.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if PersonIDRe.MatchString(s) {
		return s
	}
	return ""
}

// The host must be imdb.com itself (optionally www.), so a lookalike like
// imdb.com.evil.example cannot match.
var rePersonURL = regexp.MustCompile(`^https?://(?:www\.)?imdb\.com/name/(nm\d{6,9})/?$`)

// ClampPeople bounds, de-duplicates and caps one role's worth of people.
//
// De-duplication is within a role only (FR-004c): a writer who also directed
// appears under each heading, which is true, and twice under one, which is not.
// Identity is the IMDb id where there is one, and the name otherwise — never the
// name alone, or two different people who share one would collapse into a single
// tile carrying whichever face was resolved first.
func ClampPeople(in []Person, max int) []Person {
	if len(in) == 0 {
		return nil
	}
	out := make([]Person, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p, ok := p.Clamp()
		if !ok {
			continue
		}
		key := p.IMDbID
		if key == "" {
			key = "name:" + p.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
		if len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// How many people one title may contribute. A normal billed cast is three to
// six; the caps exist so a title cannot become a lever for a hundred outbound
// lookups (FR-007).
const (
	MaxCast = 20
	MaxCrew = 10
)

// PersonResolver is implemented by a driver whose title pages NAME people
// without identifying them — the names are free, the identity costs a request.
//
// It is an optional capability, asserted at the call site, so a driver whose API
// already carries an IMDb id per person implements nothing. The resolution is
// driven by the SERVER while assembling a response it decided to assemble: these
// calls carry the operator's stored source session, so no endpoint may let a
// client name the ref to be fetched (FR-014a).
type PersonResolver interface {
	ResolvePerson(ctx context.Context, c *Client, cfg Config, s Session, ref string) (Person, error)
}

// TitleDetail is a title with its qualities. Sendable is false for types this
// provider/version cannot send (v1: series/anime).
type TitleDetail struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Sendable  bool            `json:"sendable"`
	Qualities []QualityOption `json:"qualities"`

	// IMDbID and Plot are the source's own description of the title, so unlike
	// the two fields below they are set by the DRIVER and the handler never
	// touches them. They exist because the sheet's header metadata comes from the
	// catalog entry, and not every source puts metadata in its listing pages: a
	// site whose listing carries only a poster and a rating can still describe
	// the title on its detail page, which the driver is fetching anyway.
	//
	// Both are best-effort. A driver that cannot find them leaves them empty and
	// still returns its download options — missing metadata is not an error.
	IMDbID string `json:"imdbId,omitempty"`
	// Plot is plain text, in whatever language the source publishes; the client
	// renders it with dir="auto" rather than assuming it is left-to-right.
	Plot string `json:"plot,omitempty"`

	// Ownership and Seasons are set by the handler on the way out, never by a
	// driver — the same decoration pattern CatalogTitle.Ownership uses.
	//
	// Season detail rides on THIS response rather than a lookup of its own, and
	// that is a security decision as much as a design one: this endpoint already
	// resolves the title through the caller's own source access, so a user can
	// only ask about a title they could already open. A free-standing lookup
	// keyed by title text would have been a way around whatever narrowing applies
	// to their catalog (FR-025a, FR-025c).
	Ownership string `json:"ownership,omitempty"`
	// Seasons is present only for a series, and lists only seasons that actually
	// hold video. It never states a total or claims completeness (FR-016a).
	Seasons []SeasonPresence `json:"seasons,omitempty"`

	// Who made it (spec 0014). Cast is in the source's own billing order, never
	// re-sorted; the three crew roles are kept apart because "who is in it",
	// "who directed it", "whose show it is" and "who wrote it" are four different
	// questions.
	//
	// Every one is `omitempty`, and that is the contract rather than a tidiness:
	// an ABSENT field means the source publishes nothing for that role, which is
	// the normal case — one source leaves the director empty for most series and
	// names a creator instead. A driver must never send an empty slice to mean
	// the same thing, because the client renders a heading for a present role.
	Cast      []Person `json:"cast,omitempty"`
	Directors []Person `json:"directors,omitempty"`
	Creators  []Person `json:"creators,omitempty"`
	Writers   []Person `json:"writers,omitempty"`

	// Year as the source publishes it — a string for the same reason
	// CatalogTitle.Year is one: a series carries a range, and an ongoing series an
	// open one. Filled by the source whose detail response happens to carry it;
	// the client falls back to the year at the end of the title string.
	Year string `json:"year,omitempty"`
}

// SeasonPresence is what one season folder holds. Episodes are read from the file
// names; an empty Episodes with VideoFiles > 0 means "present, numbering
// unreadable" and MUST still render as present (FR-016b).
type SeasonPresence struct {
	Season     int   `json:"season"`
	Episodes   []int `json:"episodes"`
	VideoFiles int   `json:"videoFiles"`
}

// Provider maps generic catalog operations onto a specific site. Implementations
// are stateless: all secrets/config arrive per call so the same driver serves
// any configured instance.
type Provider interface {
	// Kind is the stable registry key (e.g. "30nama").
	Kind() string
	// DisplayName is the human name offered when an admin adds this kind.
	DisplayName() string
	// SessionFields declares the material this provider needs. The admin UI is
	// generated from it, so adding a driver needs no client change.
	SessionFields() []SessionField
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

// What a catalog title's Ownership field may say (spec 0008).
//
// Deliberately not a bool. A boolean can only say "have it / do not", and three
// of these are distinct advice: OwnershipDownloading means wait rather than send
// again, and OwnershipUnknown means nothing has been established yet and NO marker
// may be shown — a false "you have this" costs the user a film they thought was
// there, so silence is the safe answer.
const (
	OwnershipUnknown     = "unknown"
	OwnershipAbsent      = "absent"
	OwnershipOwned       = "owned"
	OwnershipDownloading = "downloading"
)

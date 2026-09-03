package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// Download-source catalog persistence (spec 0005). One admin-configured provider
// in v1: its non-secret config/status lives in source_providers; its session
// material (clearance cookie, api key, auth token, User-Agent) is sealed with the
// at-rest Cipher in source_provider_secrets and is WRITE-ONLY — never returned to
// a client, only decrypted in-process to build outbound requests. source_prefs
// holds a per-user preferred quality.

// Provider states.
const (
	SourceNotConfigured = "not_configured"
	SourceActive        = "active"
	SourceNeedsRefresh  = "needs_refresh"
	// SourceUnsubscribed: the session works, but the account has no download
	// entitlement. Deliberately distinct from SourceNeedsRefresh — telling an
	// operator to re-paste a session that is working perfectly sends them in
	// circles (spec 0007 FR-019).
	SourceUnsubscribed = "unsubscribed"
)

// SourceProvider is the non-secret provider config + status. Safe to return to
// admins as status; it contains no secret values.
type SourceProvider struct {
	ID             int64
	Kind           string
	DisplayName    string
	APIHosts       []string
	DownloadHosts  []string
	MoviesParent   string
	TVParent       string
	Enabled        bool
	State          string
	LastVerifiedAt int64
	SortOrder      int64
	// AltBase is an operator-set mirror to fall back to when the main domain is
	// unavailable (spec 1020). Empty = none.
	AltBase string
	// LastError is the last failure CATEGORY only (needs_refresh / unsubscribed /
	// unreachable). Never an upstream body, URL, or anything secret-derived.
	LastError string
}

// SourceSession is the secret material for a provider. It is sealed as JSON and
// never serialized to any client.
//
// Fields is a provider-declared bag rather than fixed columns: two sources can
// authenticate in completely different ways (one with API headers, one with a
// site login cookie), and a fixed struct would both grow a field per site and
// hand every driver every other driver's secrets.
type SourceSession struct {
	Fields    map[string]string `json:"fields"`
	UserAgent string            `json:"user_agent"`
}

// Get returns one field ("" when absent).
func (s SourceSession) Get(key string) string { return s.Fields[key] }

// legacySessionKeys are the fixed field names sealed before the bag existed.
// They are read back into the bag under the same names, so an operator who
// pasted material before this change never has to paste it again.
var legacySessionKeys = []string{
	"cf_clearance", "c_api_key", "c_token", "c_platform", "c_app_version",
}

// decodeSession reads a sealed blob in either shape. The new shape has a
// "fields" object; the old one had the keys at the top level. Anything that
// parses as neither is an error the caller must surface — unreadable material is
// retained and reported, never silently discarded (spec 0007 FR-035).
func decodeSession(plain []byte) (*SourceSession, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(plain, &raw); err != nil {
		return nil, err
	}
	out := SourceSession{Fields: map[string]string{}}
	if ua, ok := raw["user_agent"]; ok {
		_ = json.Unmarshal(ua, &out.UserAgent)
	}
	if f, ok := raw["fields"]; ok {
		if err := json.Unmarshal(f, &out.Fields); err != nil {
			return nil, err
		}
		if out.Fields == nil {
			out.Fields = map[string]string{}
		}
		return &out, nil
	}
	// Legacy shape: lift the known keys into the bag by the same name.
	for _, k := range legacySessionKeys {
		if v, ok := raw[k]; ok {
			var sv string
			if json.Unmarshal(v, &sv) == nil && sv != "" {
				out.Fields[k] = sv
			}
		}
	}
	return &out, nil
}

const providerCols = `id, kind, display_name, api_hosts, download_hosts,
	movies_parent, tv_parent, enabled, state, last_verified_at, sort_order, last_error, alt_base`

func scanProvider(sc interface{ Scan(...any) error }) (*SourceProvider, error) {
	var (
		p                 SourceProvider
		apiHosts, dlHosts string
		enabled           int
	)
	err := sc.Scan(&p.ID, &p.Kind, &p.DisplayName, &apiHosts, &dlHosts,
		&p.MoviesParent, &p.TVParent, &enabled, &p.State, &p.LastVerifiedAt,
		&p.SortOrder, &p.LastError, &p.AltBase)
	if err != nil {
		return nil, err
	}
	p.APIHosts = splitLines(apiHosts)
	p.DownloadHosts = splitLines(dlHosts)
	p.Enabled = enabled != 0
	return &p, nil
}

// ListProviders returns every configured provider in display order. Ties on
// sort_order break by id so the order is total and stable — a combined catalog
// list must not reshuffle between requests.
func (s *Store) ListProviders() ([]SourceProvider, error) {
	rows, err := s.db.Query(`SELECT ` + providerCols + `
		FROM source_providers ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceProvider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetProviderByID returns one provider (nil, nil when it does not exist).
func (s *Store) GetProviderByID(id int64) (*SourceProvider, error) {
	p, err := scanProvider(s.db.QueryRow(
		`SELECT `+providerCols+` FROM source_providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// GetProvider returns the lowest-id provider, or nil when none is configured.
// Retained for the pre-multi-source routes, which address that provider so an
// older client keeps working unchanged.
func (s *Store) GetProvider() (*SourceProvider, error) {
	p, err := scanProvider(s.db.QueryRow(
		`SELECT ` + providerCols + ` FROM source_providers ORDER BY id LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// CreateProvider inserts a new provider and returns its id.
func (s *Store) CreateProvider(p SourceProvider, now int64) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO source_providers
		  (kind, display_name, api_hosts, download_hosts, movies_parent,
		   tv_parent, enabled, state, last_verified_at, sort_order, last_error,
		   alt_base, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Kind, p.DisplayName, strings.Join(p.APIHosts, "\n"),
		strings.Join(p.DownloadHosts, "\n"), p.MoviesParent, p.TVParent,
		boolInt(p.Enabled), orDefault(p.State, SourceNotConfigured),
		p.LastVerifiedAt, p.SortOrder, p.LastError, p.AltBase, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateProvider updates one provider's non-secret config by id. It never
// touches secrets, state, or verification timestamps.
func (s *Store) UpdateProvider(p SourceProvider, now int64) error {
	_, err := s.db.Exec(`
		UPDATE source_providers SET
		  kind=?, display_name=?, api_hosts=?, download_hosts=?, movies_parent=?,
		  tv_parent=?, enabled=?, sort_order=?, alt_base=?, updated_at=?
		WHERE id=?`,
		p.Kind, p.DisplayName, strings.Join(p.APIHosts, "\n"),
		strings.Join(p.DownloadHosts, "\n"), p.MoviesParent, p.TVParent,
		boolInt(p.Enabled), p.SortOrder, p.AltBase, now, p.ID)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SaveProviderConfig upserts the LOWEST-ID provider's non-secret config and
// returns its id. Retained for the pre-multi-source admin routes; new code uses
// CreateProvider / UpdateProvider with an explicit id.
func (s *Store) SaveProviderConfig(p SourceProvider, now int64) (int64, error) {
	existing, err := s.GetProvider()
	if err != nil {
		return 0, err
	}
	if existing == nil {
		return s.CreateProvider(p, now)
	}
	p.ID = existing.ID
	p.SortOrder = existing.SortOrder
	if err := s.UpdateProvider(p, now); err != nil {
		return 0, err
	}
	return existing.ID, nil
}

// SaveProviderSession seals the session material and stores it for providerID.
func (s *Store) SaveProviderSession(providerID int64, sess SourceSession, now int64) error {
	plain, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	sealed, err := s.cipher.Seal(plain)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO source_provider_secrets (provider_id, session_enc, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET
		  session_enc = excluded.session_enc,
		  updated_at  = excluded.updated_at`,
		providerID, sealed, now)
	return err
}

// LoadProviderSession decrypts the stored session material for in-process use.
// Returns nil, nil when no secrets are stored for the provider.
func (s *Store) LoadProviderSession(providerID int64) (*SourceSession, error) {
	var sealed []byte
	err := s.db.QueryRow(
		`SELECT session_enc FROM source_provider_secrets WHERE provider_id = ?`,
		providerID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.cipher.Open(sealed)
	if err != nil {
		return nil, err
	}
	// decodeSession accepts both the current bag shape and the pre-0007 fixed
	// shape. A blob that parses as neither returns an error rather than an empty
	// session: the caller must report that the source needs attention, leaving the
	// stored material untouched (FR-035). Silently returning an empty session here
	// would look exactly like "never configured" and strand the operator.
	return decodeSession(plain)
}

// SetProviderState updates the provider's lifecycle state; lastVerifiedAt is
// applied only when > 0 (i.e. on a successful verify).
func (s *Store) SetProviderState(providerID int64, state string, lastVerifiedAt, now int64) error {
	return s.SetProviderStateErr(providerID, state, "", lastVerifiedAt, now)
}

// SetProviderStateErr also records the failure CATEGORY for admin display.
// reason must be one of the known categories — never upstream text, a URL, or
// anything derived from session material.
func (s *Store) SetProviderStateErr(providerID int64, state, reason string, lastVerifiedAt, now int64) error {
	if lastVerifiedAt > 0 {
		_, err := s.db.Exec(
			`UPDATE source_providers SET state=?, last_error=?, last_verified_at=?, updated_at=? WHERE id=?`,
			state, reason, lastVerifiedAt, now, providerID)
		return err
	}
	_, err := s.db.Exec(
		`UPDATE source_providers SET state=?, last_error=?, updated_at=? WHERE id=?`,
		state, reason, now, providerID)
	return err
}

// DeleteProvider removes the provider and (via ON DELETE CASCADE) its secrets.
func (s *Store) DeleteProvider(providerID int64) error {
	_, err := s.db.Exec(`DELETE FROM source_providers WHERE id = ?`, providerID)
	return err
}

// GetSourcePref returns the user's preferred quality ("" when unset).
func (s *Store) GetSourcePref(userID int64) (string, error) {
	var q string
	err := s.db.QueryRow(
		`SELECT preferred_quality FROM source_prefs WHERE user_id = ?`, userID).Scan(&q)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return q, err
}

// SaveSourcePref upserts the user's preferred quality.
func (s *Store) SaveSourcePref(userID int64, preferredQuality string, now int64) error {
	_, err := s.db.Exec(`
		INSERT INTO source_prefs (user_id, preferred_quality, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
		  preferred_quality = excluded.preferred_quality,
		  updated_at        = excluded.updated_at`,
		userID, preferredQuality, now)
	return err
}

// GetSourceView returns the user's saved Discover view: the facet filters (as the
// opaque JSON blob the client stored), the sort field, and its direction
// ("asc"/"desc"; "" = the app default). Empty when unset.
func (s *Store) GetSourceView(userID int64) (filters, sort, order string, err error) {
	f, so, o, _, err := s.GetSourceViewFull(userID)
	return f, so, o, err
}

// GetSourceViewFull additionally returns the selected source ("" = all sources).
// The caller normalizes an unknown or removed source back to "" — deleting a
// source must degrade a stale selection, not break it.
func (s *Store) GetSourceViewFull(userID int64) (filters, sort, order, selected string, err error) {
	err = s.db.QueryRow(
		`SELECT filters, sort, sort_order, selected_source FROM source_prefs WHERE user_id = ?`, userID).
		Scan(&filters, &sort, &order, &selected)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", nil
	}
	return filters, sort, order, selected, err
}

// SaveSourceView upserts the user's Discover view (filters JSON + sort field +
// direction), leaving the preferred quality on the same row untouched.
func (s *Store) SaveSourceView(userID int64, filters, sort, order string, now int64) error {
	return s.SaveSourceViewFull(userID, filters, sort, order, "", now)
}

// SaveSourceViewFull also stores the selected source ("" = all sources).
func (s *Store) SaveSourceViewFull(userID int64, filters, sort, order, selected string, now int64) error {
	_, err := s.db.Exec(`
		INSERT INTO source_prefs (user_id, filters, sort, sort_order, selected_source, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
		  filters         = excluded.filters,
		  sort            = excluded.sort,
		  sort_order      = excluded.sort_order,
		  selected_source = excluded.selected_source,
		  updated_at      = excluded.updated_at`,
		userID, filters, sort, order, selected, now)
	return err
}

// SourceDownload is the catalog metadata remembered for a Discover send, keyed by
// its destination subfolder, so the Tasks list can label the download.
type SourceDownload struct {
	Destination string
	MediaType   string // movie / series / anime
	Title       string
	Year        string
	IMDbScore   float64
	PosterURL   string // public catalog poster image (spec 1016); "" when unknown
	CatalogID   string // catalog title id for "Open in Discover" (spec 1016); "" when unknown
	OwnerID     int64  // who sent it (0 = unknown)
	OwnerName   string // their username (joined; "" when unknown)
}

// normDest strips leading/trailing slashes so a stored destination matches the
// (share-relative, variably-slashed) destination DSM reports on each task.
func normDest(dest string) string { return strings.Trim(strings.TrimSpace(dest), "/") }

// SaveSourceDownload upserts the catalog metadata for a title's destination
// folder (a re-send overwrites it).
func (s *Store) SaveSourceDownload(d SourceDownload, now int64) error {
	var owner any // NULL when unknown, so ON DELETE SET NULL applies
	if d.OwnerID != 0 {
		owner = d.OwnerID
	}
	_, err := s.db.Exec(`
		INSERT INTO source_downloads (destination, media_type, title, year, imdb_score, poster_url, catalog_id, owner_user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(destination) DO UPDATE SET
			media_type    = excluded.media_type,
			title         = excluded.title,
			year          = excluded.year,
			imdb_score    = excluded.imdb_score,
			poster_url    = excluded.poster_url,
			catalog_id    = excluded.catalog_id,
			owner_user_id = excluded.owner_user_id,
			created_at    = excluded.created_at`,
		normDest(d.Destination), d.MediaType, d.Title, d.Year, d.IMDbScore, d.PosterURL, d.CatalogID, owner, now)
	return err
}

// SourceDownloads returns all remembered Discover-send metadata (incl. who sent
// it), keyed by the normalized destination folder, for joining onto the task list.
func (s *Store) SourceDownloads() (map[string]SourceDownload, error) {
	rows, err := s.db.Query(`
		SELECT sd.destination, sd.media_type, sd.title, sd.year, sd.imdb_score,
		       sd.poster_url, sd.catalog_id,
		       COALESCE(sd.owner_user_id, 0), COALESCE(u.username, '')
		FROM source_downloads sd
		LEFT JOIN users u ON u.id = sd.owner_user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SourceDownload{}
	for rows.Next() {
		var d SourceDownload
		if err := rows.Scan(&d.Destination, &d.MediaType, &d.Title, &d.Year, &d.IMDbScore, &d.PosterURL, &d.CatalogID, &d.OwnerID, &d.OwnerName); err != nil {
			return nil, err
		}
		out[d.Destination] = d
	}
	return out, rows.Err()
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

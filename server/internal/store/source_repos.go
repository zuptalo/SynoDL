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
}

// SourceSession is the secret material for a provider. It is sealed as JSON and
// never serialized to any client.
type SourceSession struct {
	CFClearance string `json:"cf_clearance"`
	CAPIKey     string `json:"c_api_key"`
	CToken      string `json:"c_token"`
	UserAgent   string `json:"user_agent"`
	CPlatform   string `json:"c_platform"`
	CAppVersion string `json:"c_app_version"`
}

// GetProvider returns the single configured provider (v1 treats it as a
// singleton: the lowest-id row). Returns nil, nil when none is configured.
func (s *Store) GetProvider() (*SourceProvider, error) {
	row := s.db.QueryRow(`
		SELECT id, kind, display_name, api_hosts, download_hosts, movies_parent,
		       tv_parent, enabled, state, last_verified_at
		FROM source_providers ORDER BY id LIMIT 1`)
	var (
		p                 SourceProvider
		apiHosts, dlHosts string
		enabled           int
	)
	err := row.Scan(&p.ID, &p.Kind, &p.DisplayName, &apiHosts, &dlHosts,
		&p.MoviesParent, &p.TVParent, &enabled, &p.State, &p.LastVerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.APIHosts = splitLines(apiHosts)
	p.DownloadHosts = splitLines(dlHosts)
	p.Enabled = enabled != 0
	return &p, nil
}

// SaveProviderConfig upserts the singleton provider's non-secret config and
// returns its id. It never touches secrets or state timestamps beyond `state`.
func (s *Store) SaveProviderConfig(p SourceProvider, now int64) (int64, error) {
	existing, err := s.GetProvider()
	if err != nil {
		return 0, err
	}
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	apiHosts := strings.Join(p.APIHosts, "\n")
	dlHosts := strings.Join(p.DownloadHosts, "\n")
	if existing == nil {
		res, err := s.db.Exec(`
			INSERT INTO source_providers
			  (kind, display_name, api_hosts, download_hosts, movies_parent,
			   tv_parent, enabled, state, last_verified_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Kind, p.DisplayName, apiHosts, dlHosts, p.MoviesParent, p.TVParent,
			enabled, orDefault(p.State, SourceNotConfigured), p.LastVerifiedAt, now, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err = s.db.Exec(`
		UPDATE source_providers SET
		  kind=?, display_name=?, api_hosts=?, download_hosts=?, movies_parent=?,
		  tv_parent=?, enabled=?, updated_at=?
		WHERE id=?`,
		p.Kind, p.DisplayName, apiHosts, dlHosts, p.MoviesParent, p.TVParent,
		enabled, now, existing.ID)
	if err != nil {
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
	var sess SourceSession
	if err := json.Unmarshal(plain, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// SetProviderState updates the provider's lifecycle state; lastVerifiedAt is
// applied only when > 0 (i.e. on a successful verify).
func (s *Store) SetProviderState(providerID int64, state string, lastVerifiedAt, now int64) error {
	if lastVerifiedAt > 0 {
		_, err := s.db.Exec(
			`UPDATE source_providers SET state=?, last_verified_at=?, updated_at=? WHERE id=?`,
			state, lastVerifiedAt, now, providerID)
		return err
	}
	_, err := s.db.Exec(
		`UPDATE source_providers SET state=?, updated_at=? WHERE id=?`, state, now, providerID)
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
	err = s.db.QueryRow(
		`SELECT filters, sort, sort_order FROM source_prefs WHERE user_id = ?`, userID).
		Scan(&filters, &sort, &order)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", nil
	}
	return filters, sort, order, err
}

// SaveSourceView upserts the user's Discover view (filters JSON + sort field +
// direction), leaving the preferred quality on the same row untouched.
func (s *Store) SaveSourceView(userID int64, filters, sort, order string, now int64) error {
	_, err := s.db.Exec(`
		INSERT INTO source_prefs (user_id, filters, sort, sort_order, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
		  filters    = excluded.filters,
		  sort       = excluded.sort,
		  sort_order = excluded.sort_order,
		  updated_at = excluded.updated_at`,
		userID, filters, sort, order, now)
	return err
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

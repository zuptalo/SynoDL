package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned by Get* lookups when the row is absent.
var ErrNotFound = errors.New("store: not found")

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- operator_config (singleton) ------------------------------------------

// OperatorConfig is the single NAS connection + public URL captured by the
// setup wizard. NASPassword is plaintext in memory only; it is encrypted at
// rest in nas_password_enc.
type OperatorConfig struct {
	PublicURL    string
	NASAddress   string
	NASPort      int
	NASTLSVerify bool
	NASAccount   string
	NASPassword  string
	NASUses2FA   bool
	CreatedAt    int64
	UpdatedAt    int64
}

// HasOperatorConfig reports whether setup has been completed (drives the wizard
// gate).
func (s *Store) HasOperatorConfig() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM operator_config WHERE id = 1`).Scan(&n)
	return n > 0, err
}

// DeleteOperatorConfig removes the singleton (used to roll back a failed setup
// so the wizard runs again rather than leaving a broken connection stored).
func (s *Store) DeleteOperatorConfig() error {
	_, err := s.db.Exec(`DELETE FROM operator_config WHERE id = 1`)
	return err
}

// SaveNASSID caches the current DSM session id, encrypted, so a restart can
// resume it instead of forcing a re-login (which a 2FA account can't do
// unattended). No-op if setup hasn't run.
func (s *Store) SaveNASSID(sid string) error {
	enc, err := s.cipher.Seal([]byte(sid))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE operator_config SET nas_sid_enc = ? WHERE id = 1`, enc)
	return err
}

// GetNASSID returns the cached session id ("" if none/undecryptable — the
// caller just re-authenticates).
func (s *Store) GetNASSID() (string, error) {
	var enc []byte
	err := s.db.QueryRow(`SELECT nas_sid_enc FROM operator_config WHERE id = 1`).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) || len(enc) == 0 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	pt, err := s.cipher.Open(enc)
	if err != nil {
		return "", nil // stale/wrong-key cache: treat as none
	}
	return string(pt), nil
}

// ClearNASSID drops the cached session id (on expiry).
func (s *Store) ClearNASSID() error {
	_, err := s.db.Exec(`UPDATE operator_config SET nas_sid_enc = NULL WHERE id = 1`)
	return err
}

// SaveOperatorConfig upserts the singleton, encrypting the NAS password.
func (s *Store) SaveOperatorConfig(c OperatorConfig) error {
	enc, err := s.cipher.Seal([]byte(c.NASPassword))
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`
		INSERT INTO operator_config
			(id, public_url, nas_address, nas_port, nas_tls_verify, nas_account, nas_password_enc, nas_uses_2fa, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			public_url = excluded.public_url, nas_address = excluded.nas_address, nas_port = excluded.nas_port,
			nas_tls_verify = excluded.nas_tls_verify, nas_account = excluded.nas_account,
			nas_password_enc = excluded.nas_password_enc, nas_uses_2fa = excluded.nas_uses_2fa,
			updated_at = excluded.updated_at`,
		c.PublicURL, c.NASAddress, c.NASPort, boolToInt(c.NASTLSVerify), c.NASAccount, enc, boolToInt(c.NASUses2FA), now, now)
	return err
}

// GetOperatorConfig returns the singleton with the NAS password decrypted, or
// ErrNotFound if setup hasn't run. A decrypt failure (wrong/missing SECRETS_KEY)
// surfaces as an error — the caller turns it into a boot failure, never a reset.
func (s *Store) GetOperatorConfig() (*OperatorConfig, error) {
	var (
		c          OperatorConfig
		enc        []byte
		tls, twofa int
	)
	err := s.db.QueryRow(`
		SELECT public_url, nas_address, nas_port, nas_tls_verify, nas_account, nas_password_enc, nas_uses_2fa, created_at, updated_at
		FROM operator_config WHERE id = 1`).
		Scan(&c.PublicURL, &c.NASAddress, &c.NASPort, &tls, &c.NASAccount, &enc, &twofa, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.NASTLSVerify = tls != 0
	c.NASUses2FA = twofa != 0
	if len(enc) > 0 {
		pt, err := s.cipher.Open(enc)
		if err != nil {
			return nil, err
		}
		c.NASPassword = string(pt)
	}
	return &c, nil
}

// ---- users ----------------------------------------------------------------

// User is a SynoDL account (no relation to any NAS account). PasswordHash is a
// salted hash produced by internal/auth; the plaintext password is never stored.
type User struct {
	ID            int64
	Username      string
	PasswordHash  string
	IsAdmin       bool
	IsEnabled     bool
	ContentRating string
	CreatedAt     int64
	UpdatedAt     int64
}

// CreateUser inserts a user and returns its id. Username uniqueness is
// case-insensitive (enforced by a NOCASE index).
func (s *Store) CreateUser(username, passwordHash string, isAdmin bool) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, is_admin, is_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, 1, ?, ?)`,
		username, passwordHash, boolToInt(isAdmin), now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var admin, enabled int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &admin, &enabled, &u.ContentRating, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.IsAdmin = admin != 0
	u.IsEnabled = enabled != 0
	return &u, nil
}

const userCols = `id, username, password_hash, is_admin, is_enabled, content_rating, created_at, updated_at`

// GetUserByUsername looks up a user case-insensitively (for login).
func (s *Store) GetUserByUsername(username string) (*User, error) {
	u, err := scanUser(s.db.QueryRow(
		`SELECT `+userCols+` FROM users WHERE username = ? COLLATE NOCASE`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) GetUserByID(id int64) (*User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// SetUserEnabled toggles a user; disabling also drops their sessions (immediate
// lockout) via the caller pairing with DeleteUserSessions, or here directly.
func (s *Store) SetUserEnabled(id int64, enabled bool) error {
	if _, err := s.db.Exec(`UPDATE users SET is_enabled = ?, updated_at = ? WHERE id = ?`,
		boolToInt(enabled), time.Now().Unix(), id); err != nil {
		return err
	}
	if !enabled {
		return s.DeleteUserSessions(id)
	}
	return nil
}

// SetUserAdmin toggles a user's admin flag.
func (s *Store) SetUserAdmin(id int64, isAdmin bool) error {
	_, err := s.db.Exec(`UPDATE users SET is_admin = ?, updated_at = ? WHERE id = ?`,
		boolToInt(isAdmin), time.Now().Unix(), id)
	return err
}

func (s *Store) SetUserPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, time.Now().Unix(), id)
	return err
}

// SetUserContentRating caps the ratings a user may see in the catalog. Empty
// clears the cap (unrestricted).
func (s *Store) SetUserContentRating(id int64, rating string) error {
	_, err := s.db.Exec(`UPDATE users SET content_rating = ?, updated_at = ? WHERE id = ?`,
		rating, time.Now().Unix(), id)
	return err
}

// DeleteUser removes a user; ON DELETE CASCADE clears their sessions, grants,
// and subscriptions.
func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// ---- folder grants --------------------------------------------------------

// ListFolderGrants returns a user's allowed NAS folder paths (normalized,
// without leading slashes).
func (s *Store) ListFolderGrants(userID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT folder_path FROM folder_grants WHERE user_id = ? ORDER BY folder_path`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetFolderGrants replaces a user's grants with the given paths (deduped by the
// UNIQUE index; the caller normalizes them).
func (s *Store) SetFolderGrants(userID int64, paths []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM folder_grants WHERE user_id = ?`, userID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, p := range paths {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO folder_grants (user_id, folder_path) VALUES (?, ?)`, userID, p); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---- sessions -------------------------------------------------------------

// CreateSession records an opaque session by the HASH of its token (the raw
// token is never stored), tied to a user, with an expiry.
func (s *Store) CreateSession(tokenHash string, userID, expiresAt int64) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, time.Now().Unix(), expiresAt)
	return err
}

// UserForSession returns the enabled user behind a live (unexpired) session, or
// ErrNotFound. Disabled users and expired sessions do not resolve.
func (s *Store) UserForSession(tokenHash string, now int64) (*User, error) {
	u, err := scanUser(s.db.QueryRow(
		`SELECT `+userColsPrefixed+` FROM users u JOIN sessions se ON se.user_id = u.id
		 WHERE se.token_hash = ? AND se.expires_at > ? AND u.is_enabled = 1`, tokenHash, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

const userColsPrefixed = `u.id, u.username, u.password_hash, u.is_admin, u.is_enabled, u.content_rating, u.created_at, u.updated_at`

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) DeleteExpiredSessions(now int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now)
	return err
}

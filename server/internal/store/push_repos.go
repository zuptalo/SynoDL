package store

import (
	"database/sql"
	"errors"
)

// ---- instance (VAPID keys + app-update high-water mark) --------------------

// VAPID holds the instance's Web Push signing keys. Private is decrypted.
type VAPID struct {
	Public  string
	Private string
	Subject string
}

// GetVAPID returns the stored VAPID keys, or ErrNotFound if none generated yet.
func (s *Store) GetVAPID() (*VAPID, error) {
	var (
		v   VAPID
		enc []byte
	)
	err := s.db.QueryRow(`SELECT vapid_public, vapid_private_enc, vapid_subject FROM instance WHERE id = 1`).
		Scan(&v.Public, &enc, &v.Subject)
	if errors.Is(err, sql.ErrNoRows) || v.Public == "" {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(enc) > 0 {
		pt, err := s.cipher.Open(enc)
		if err != nil {
			return nil, err
		}
		v.Private = string(pt)
	}
	return &v, nil
}

// SaveVAPID stores the keypair, encrypting the private key at rest.
func (s *Store) SaveVAPID(v VAPID) error {
	enc, err := s.cipher.Seal([]byte(v.Private))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO instance (id, vapid_public, vapid_private_enc, vapid_subject)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			vapid_public = excluded.vapid_public,
			vapid_private_enc = excluded.vapid_private_enc,
			vapid_subject = excluded.vapid_subject`,
		v.Public, enc, v.Subject)
	return err
}

// LastVersionNotified / SetLastVersionNotified track the app version last pushed
// as an update notice, so a redeploy notifies once.
func (s *Store) LastVersionNotified() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT last_version_notified FROM instance WHERE id = 1`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetLastVersionNotified(version string) error {
	_, err := s.db.Exec(`
		INSERT INTO instance (id, last_version_notified) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET last_version_notified = excluded.last_version_notified`, version)
	return err
}

// ---- push subscriptions ---------------------------------------------------

// Subscription is one device's Web Push endpoint.
type Subscription struct {
	ID       int64
	UserID   int64
	Endpoint string
	P256dh   string
	Auth     string
	OptedIn  bool
}

// SaveSubscription upserts a device subscription (keyed by endpoint).
func (s *Store) SaveSubscription(userID int64, endpoint, p256dh, authKey string, optedIn bool) error {
	_, err := s.db.Exec(`
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, opted_in, created_at)
		VALUES (?, ?, ?, ?, ?, strftime('%s','now'))
		ON CONFLICT(endpoint) DO UPDATE SET
			user_id = excluded.user_id, p256dh = excluded.p256dh,
			auth = excluded.auth, opted_in = excluded.opted_in`,
		userID, endpoint, p256dh, authKey, boolToInt(optedIn))
	return err
}

func (s *Store) DeleteSubscription(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

func scanSubs(rows *sql.Rows) ([]Subscription, error) {
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var sub Subscription
		var opted int
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &opted); err != nil {
			return nil, err
		}
		sub.OptedIn = opted != 0
		out = append(out, sub)
	}
	return out, rows.Err()
}

const subCols = `id, user_id, endpoint, p256dh, auth, opted_in`

// OptedInSubscriptions returns every opted-in device (home-scale fan-out target).
func (s *Store) OptedInSubscriptions() ([]Subscription, error) {
	rows, err := s.db.Query(`SELECT ` + subCols + ` FROM push_subscriptions WHERE opted_in = 1`)
	if err != nil {
		return nil, err
	}
	return scanSubs(rows)
}

// ---- watched tasks (completion detection) ---------------------------------

// GetWatched returns a task's last-seen status and whether a completion push has
// been sent, plus whether we've seen it before.
func (s *Store) GetWatched(taskID string) (lastStatus string, notified, found bool, err error) {
	var n int
	err = s.db.QueryRow(`SELECT last_status, notified FROM watched_tasks WHERE nas_task_id = ?`, taskID).
		Scan(&lastStatus, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	return lastStatus, n != 0, true, nil
}

// UpsertWatched records a task's current status + notified flag.
func (s *Store) UpsertWatched(taskID, status string, notified bool) error {
	_, err := s.db.Exec(`
		INSERT INTO watched_tasks (nas_task_id, last_status, notified, first_seen, updated_at)
		VALUES (?, ?, ?, strftime('%s','now'), strftime('%s','now'))
		ON CONFLICT(nas_task_id) DO UPDATE SET
			last_status = excluded.last_status, notified = excluded.notified,
			updated_at = excluded.updated_at`,
		taskID, status, boolToInt(notified))
	return err
}

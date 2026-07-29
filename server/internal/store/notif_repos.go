package store

import (
	"database/sql"
	"errors"
)

// Per-user notification preferences and task-ownership claims (spec 1004).

// NotificationPrefs is a user's choice of which download events notify them and
// whether that covers every user's tasks or only their own.
type NotificationPrefs struct {
	NotifyAdded     bool
	NotifyCompleted bool
	NotifyFailed    bool
	Scope           string // "own" | "any"
}

// DefaultNotificationPrefs is what a user gets before they change anything:
// completions and failures for their OWN tasks (added is off — you just added
// it yourself; it becomes useful with the "any" scope).
func DefaultNotificationPrefs() NotificationPrefs {
	return NotificationPrefs{NotifyAdded: false, NotifyCompleted: true, NotifyFailed: true, Scope: "own"}
}

// GetNotificationPrefs returns the user's prefs, or the defaults when they have
// no row yet (never an error for "not found").
func (s *Store) GetNotificationPrefs(userID int64) (NotificationPrefs, error) {
	row := s.db.QueryRow(
		`SELECT notify_added, notify_completed, notify_failed, scope FROM notification_prefs WHERE user_id = ?`,
		userID)
	var p NotificationPrefs
	var added, completed, failed int
	err := row.Scan(&added, &completed, &failed, &p.Scope)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultNotificationPrefs(), nil
	}
	if err != nil {
		return NotificationPrefs{}, err
	}
	p.NotifyAdded, p.NotifyCompleted, p.NotifyFailed = added == 1, completed == 1, failed == 1
	return p, nil
}

// SaveNotificationPrefs upserts a user's preferences.
func (s *Store) SaveNotificationPrefs(userID int64, p NotificationPrefs, now int64) error {
	scope := p.Scope
	if scope != "any" {
		scope = "own"
	}
	_, err := s.db.Exec(`
		INSERT INTO notification_prefs (user_id, notify_added, notify_completed, notify_failed, scope, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			notify_added = excluded.notify_added,
			notify_completed = excluded.notify_completed,
			notify_failed = excluded.notify_failed,
			scope = excluded.scope,
			updated_at = excluded.updated_at`,
		userID, b2i(p.NotifyAdded), b2i(p.NotifyCompleted), b2i(p.NotifyFailed), scope, now)
	return err
}

// AddTaskClaim records that userID created a task whose DSM title will be
// nameHint, so the watcher can attribute the task when it first appears (DSM's
// create call returns no id, so ownership can't be recorded directly).
func (s *Store) AddTaskClaim(userID int64, nameHint string, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO task_claims (user_id, name_hint, created_at) VALUES (?, ?, ?)`,
		userID, nameHint, now)
	return err
}

// ClaimOwner consumes the oldest still-pending claim matching nameHint created
// at/after `since`, returning its user id. It deletes the claim so a task is
// attributed once. ok is false when nothing matches.
func (s *Store) ClaimOwner(nameHint string, since int64) (userID int64, ok bool, err error) {
	row := s.db.QueryRow(
		`SELECT id, user_id FROM task_claims WHERE name_hint = ? AND created_at >= ? ORDER BY created_at LIMIT 1`,
		nameHint, since)
	var id int64
	if e := row.Scan(&id, &userID); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, e
	}
	if _, e := s.db.Exec(`DELETE FROM task_claims WHERE id = ?`, id); e != nil {
		return 0, false, e
	}
	return userID, true, nil
}

// TaskOwner is a task's attributed creator (spec 1013 — task ownership).
type TaskOwner struct {
	UserID   int64
	Username string
}

// TaskOwners maps every attributed DSM task id to its creator (id + username),
// so the Tasks list can label each row "added by <user>" and filter by owner.
// Unattributed tasks (no claim matched, or created outside SynoDL) are absent.
func (s *Store) TaskOwners() (map[string]TaskOwner, error) {
	rows, err := s.db.Query(`
		SELECT wt.nas_task_id, u.id, u.username
		FROM watched_tasks wt
		JOIN users u ON u.id = wt.owner_user_id
		WHERE wt.owner_user_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TaskOwner{}
	for rows.Next() {
		var id string
		var o TaskOwner
		if err := rows.Scan(&id, &o.UserID, &o.Username); err != nil {
			return nil, err
		}
		out[id] = o
	}
	return out, rows.Err()
}

// PendingClaimNames returns the DSM task names a user has un-consumed claims for,
// created at/after `since`. The Tasks list treats a task matching one of these as
// already the user's own during the brief window between creating it and the
// watcher attributing it (so a fresh download shows up immediately, not after the
// next poll).
func (s *Store) PendingClaimNames(userID, since int64) (map[string]bool, error) {
	rows, err := s.db.Query(
		`SELECT name_hint FROM task_claims WHERE user_id = ? AND created_at >= ?`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

// EffectiveNotificationScope resolves the scope that governs BOTH which tasks a
// user sees in the list and which they're notified about. A non-admin is always
// "own" (their own downloads only). An admin defaults to "any" (everyone's) so
// they oversee the instance out of the box, but may opt down to "own".
func (s *Store) EffectiveNotificationScope(userID int64, isAdmin bool) (string, error) {
	if !isAdmin {
		return "own", nil
	}
	var scope string
	err := s.db.QueryRow(`SELECT scope FROM notification_prefs WHERE user_id = ?`, userID).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return "any", nil
	}
	if err != nil {
		return "", err
	}
	if scope != "any" {
		scope = "own"
	}
	return scope, nil
}

// SetWatchedOwner stamps the attributed owner on a watched task.
func (s *Store) SetWatchedOwner(taskID string, userID int64) error {
	_, err := s.db.Exec(`UPDATE watched_tasks SET owner_user_id = ? WHERE nas_task_id = ?`, userID, taskID)
	return err
}

// GetWatchedOwner returns the attributed owner of a task (ok=false if none).
func (s *Store) GetWatchedOwner(taskID string) (userID int64, ok bool, err error) {
	row := s.db.QueryRow(`SELECT owner_user_id FROM watched_tasks WHERE nas_task_id = ?`, taskID)
	var owner *int64
	if e := row.Scan(&owner); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, e
	}
	if owner == nil {
		return 0, false, nil
	}
	return *owner, true, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Per-user destination preferences: a default download folder and an ordered
// list of favorite folders (spec 1011). Stored server-side so they survive app
// closure and sync across a user's sessions.

// GetDestinationPrefs returns the user's default destination and favorites
// (empty defaults when they have no row yet).
func (s *Store) GetDestinationPrefs(userID int64) (defaultDest string, favorites []string, err error) {
	var favJoined string
	row := s.db.QueryRow(
		`SELECT default_dest, favorites FROM destination_prefs WHERE user_id = ?`, userID)
	e := row.Scan(&defaultDest, &favJoined)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil, nil
	}
	if e != nil {
		return "", nil, e
	}
	return defaultDest, splitLines(favJoined), nil
}

// SaveDestinationPrefs upserts the user's default + favorites.
func (s *Store) SaveDestinationPrefs(userID int64, defaultDest string, favorites []string, now int64) error {
	_, err := s.db.Exec(`
		INSERT INTO destination_prefs (user_id, default_dest, favorites, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			default_dest = excluded.default_dest,
			favorites = excluded.favorites,
			updated_at = excluded.updated_at`,
		userID, defaultDest, strings.Join(favorites, "\n"), now)
	return err
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0)
	for _, p := range strings.Split(s, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

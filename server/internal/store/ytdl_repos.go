package store

import "time"

// Failed YouTube downloads (spec 0012).
//
// Successful downloads are deliberately NOT recorded: the files they produced
// are their own record, and mirroring live job state into SQLite would drift the
// moment a job outlives a server restart. Only failure needs remembering,
// because a swept job otherwise leaves no trace that anything went wrong.

// YtdlFailure is one download that did not succeed.
type YtdlFailure struct {
	RequestID string
	UserID    *int64
	SourceURL string
	Mode      string
	Scope     string
	Reason    string
	FailedAt  int64
}

// maxYtdlFailures bounds the table. A failure log is a courtesy, not an audit
// trail, and an unbounded one on a home NAS is a slow leak (FR-024a).
const maxYtdlFailures = 200

// RecordYtdlFailure stores a failed download.
//
// Idempotent by request id: state is POLLED, so the same failed job will be
// observed on every refresh until it is swept. Re-observing must not create
// duplicates, and must not keep moving the timestamp — the first sighting is
// when it failed.
func (s *Store) RecordYtdlFailure(f YtdlFailure) error {
	if f.FailedAt == 0 {
		f.FailedAt = time.Now().Unix()
	}
	if _, err := s.db.Exec(
		`INSERT INTO ytdl_failures (request_id, user_id, source_url, mode, scope, reason, failed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(request_id) DO NOTHING`,
		f.RequestID, f.UserID, f.SourceURL, f.Mode, f.Scope, f.Reason, f.FailedAt,
	); err != nil {
		return err
	}
	return s.pruneYtdlFailures()
}

// pruneYtdlFailures keeps only the most recent rows.
func (s *Store) pruneYtdlFailures() error {
	_, err := s.db.Exec(
		`DELETE FROM ytdl_failures WHERE request_id NOT IN (
		   SELECT request_id FROM ytdl_failures ORDER BY failed_at DESC, rowid DESC LIMIT ?
		 )`, maxYtdlFailures)
	return err
}

// ListYtdlFailures returns stored failures, newest first.
func (s *Store) ListYtdlFailures() ([]YtdlFailure, error) {
	rows, err := s.db.Query(
		`SELECT request_id, user_id, source_url, mode, scope, reason, failed_at
		   FROM ytdl_failures ORDER BY failed_at DESC, rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []YtdlFailure
	for rows.Next() {
		var f YtdlFailure
		if err := rows.Scan(&f.RequestID, &f.UserID, &f.SourceURL, &f.Mode, &f.Scope, &f.Reason, &f.FailedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteYtdlFailure dismisses one stored failure. Dismissing removes the record
// of a download, never a downloaded file.
func (s *Store) DeleteYtdlFailure(requestID string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM ytdl_failures WHERE request_id = ?`, requestID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Ownership-scoped variants (spec 0013, FR-007/FR-008).
//
// The filtering happens in SQL rather than after loading every row, for the same
// reason the list is paged: history is unbounded, so "load it all and discard
// most of it" gets slower forever. It is also the safer shape — a row the caller
// may not see never enters the process at all, so it cannot leak through a count
// or a log line on the way past.
//
// A row whose user_id is NULL belonged to a deleted account (FR-006d). It is
// visible to admins only: there is nobody left for it to belong to.

// ListYtdlFailuresFor returns stored failures the given user may see.
func (s *Store) ListYtdlFailuresFor(userID int64, isAdmin bool) ([]YtdlFailure, error) {
	if isAdmin {
		return s.ListYtdlFailures()
	}
	rows, err := s.db.Query(
		`SELECT request_id, user_id, source_url, mode, scope, reason, failed_at
		   FROM ytdl_failures WHERE user_id = ? ORDER BY failed_at DESC, rowid DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []YtdlFailure
	for rows.Next() {
		var f YtdlFailure
		if err := rows.Scan(&f.RequestID, &f.UserID, &f.SourceURL, &f.Mode, &f.Scope, &f.Reason, &f.FailedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteYtdlFailureFor dismisses one stored failure, if the user may see it.
//
// Ownership is part of the WHERE clause rather than a check before it: that way
// "not yours" and "not there" produce the same answer — no rows affected — and
// the handler cannot accidentally distinguish them (FR-008).
func (s *Store) DeleteYtdlFailureFor(requestID string, userID int64, isAdmin bool) (bool, error) {
	if isAdmin {
		return s.DeleteYtdlFailure(requestID)
	}
	res, err := s.db.Exec(
		`DELETE FROM ytdl_failures WHERE request_id = ? AND user_id = ?`, requestID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

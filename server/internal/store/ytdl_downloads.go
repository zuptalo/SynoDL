package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Every YouTube download, durable (spec 0013).
//
// The split this file lives on either side of is Principle III's: what was
// ASKED FOR and how it ENDED is durable here; what a worker is doing right now
// is derived from the orchestrator and never written down. There is no progress
// column, and its absence is the design rather than an omission.

// YtdlKind distinguishes a plain download from a playlist/channel and its items.
type YtdlKind string

const (
	YtdlKindSingle YtdlKind = "single" // a directly submitted link
	YtdlKindGroup  YtdlKind = "group"  // a playlist or channel
	YtdlKindItem   YtdlKind = "item"   // one entry of a group
)

// YtdlOrigin says how a download came to exist. It is what lets a pasted link be
// admitted ahead of the same user's bulk-expanded work (FR-022c).
type YtdlOrigin string

const (
	YtdlOriginDirect   YtdlOrigin = "direct"
	YtdlOriginExpanded YtdlOrigin = "expanded"
)

// YtdlDownload is one download's durable record.
type YtdlDownload struct {
	RequestID  string
	ParentID   string // empty unless this is an item
	Kind       YtdlKind
	UserID     *int64 // nil once the account is deleted (FR-006d)
	SourceURL  string
	VideoID    string // stable identity for the already-held check (FR-016b)
	Mode       string
	Scope      string
	State      string
	Title      string
	Uploader   string
	Artwork    string
	GroupName  string // the playlist or channel name, already sanitised
	Origin     YtdlOrigin
	HasLyrics  *bool
	LyricsLang string
	Reason     string
	Attempts   int
	QueueSeq   int64
	CreatedAt  int64
	FinishedAt *int64 // nil until final — absent, never zero (FR-033)
}

const ytdlCols = `request_id, parent_id, kind, user_id, source_url, video_id, mode, scope, state,
	title, uploader, artwork, group_name, origin, has_lyrics, lyrics_lang, reason, attempts,
	queue_seq, created_at, finished_at`

func scanYtdlDownload(sc interface{ Scan(...any) error }) (YtdlDownload, error) {
	var d YtdlDownload
	var parent sql.NullString
	var kind, origin string
	err := sc.Scan(&d.RequestID, &parent, &kind, &d.UserID, &d.SourceURL, &d.VideoID,
		&d.Mode, &d.Scope, &d.State, &d.Title, &d.Uploader, &d.Artwork, &d.GroupName,
		&origin, &d.HasLyrics, &d.LyricsLang, &d.Reason, &d.Attempts,
		&d.QueueSeq, &d.CreatedAt, &d.FinishedAt)
	if err != nil {
		return YtdlDownload{}, err
	}
	d.ParentID = parent.String
	d.Kind = YtdlKind(kind)
	d.Origin = YtdlOrigin(origin)
	return d, nil
}

// CreateYtdlDownload inserts one download.
//
// queue_seq is assigned here rather than by the caller: it is the tiebreak
// inside fair-share ordering, so it has to be monotonic across everything in the
// table, which only the table can guarantee.
func (s *Store) CreateYtdlDownload(d YtdlDownload) error {
	if d.CreatedAt == 0 {
		d.CreatedAt = time.Now().Unix()
	}
	if d.Kind == "" {
		d.Kind = YtdlKindSingle
	}
	if d.Origin == "" {
		d.Origin = YtdlOriginDirect
	}
	if d.Attempts == 0 {
		d.Attempts = 1
	}
	var parent any
	if d.ParentID != "" {
		parent = d.ParentID
	}
	_, err := s.db.Exec(
		`INSERT INTO ytdl_downloads (`+ytdlCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		         COALESCE((SELECT MAX(queue_seq) FROM ytdl_downloads), 0) + 1, ?, ?)`,
		d.RequestID, parent, string(d.Kind), d.UserID, d.SourceURL, d.VideoID,
		d.Mode, d.Scope, d.State, d.Title, d.Uploader, d.Artwork, d.GroupName,
		string(d.Origin), d.HasLyrics, d.LyricsLang, d.Reason, d.Attempts,
		d.CreatedAt, d.FinishedAt)
	return err
}

// GetYtdlDownload returns one download, or ErrNotFound.
func (s *Store) GetYtdlDownload(requestID string) (YtdlDownload, error) {
	d, err := scanYtdlDownload(s.db.QueryRow(
		`SELECT `+ytdlCols+` FROM ytdl_downloads WHERE request_id = ?`, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return YtdlDownload{}, ErrNotFound
	}
	return d, err
}

// ListYtdlDownloads returns a page of top-level downloads the user may see.
//
// Items of a group are excluded (FR-019b): a group is one row in the list, and
// its items live behind it. Paged by (created_at, request_id) rather than by
// OFFSET, because history is unbounded and OFFSET re-walks everything it skips.
func (s *Store) ListYtdlDownloads(userID int64, isAdmin bool, cursor string, limit int) ([]YtdlDownload, string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := []string{"parent_id IS NULL"}
	args := []any{}
	if !isAdmin {
		where = append(where, "user_id = ?")
		args = append(args, userID)
	}
	if cursor != "" {
		createdAt, id, ok := decodeYtdlCursor(cursor)
		if ok {
			where = append(where, "(created_at < ? OR (created_at = ? AND request_id < ?))")
			args = append(args, createdAt, createdAt, id)
		}
	}
	// One more than asked for, so "is there another page?" needs no COUNT.
	args = append(args, limit+1)

	rows, err := s.db.Query(
		`SELECT `+ytdlCols+` FROM ytdl_downloads WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, request_id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []YtdlDownload
	for rows.Next() {
		d, err := scanYtdlDownload(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var next string
	if len(out) > limit {
		last := out[limit-1]
		next = encodeYtdlCursor(last.CreatedAt, last.RequestID)
		out = out[:limit]
	}
	return out, next, nil
}

// ListYtdlItems returns a group's items, oldest first — the order they were
// found in, which is the order the source published them.
func (s *Store) ListYtdlItems(parentID string, cursor string, limit int) ([]YtdlDownload, string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{parentID}
	extra := ""
	if cursor != "" {
		if seq, ok := decodeYtdlSeqCursor(cursor); ok {
			extra = " AND queue_seq > ?"
			args = append(args, seq)
		}
	}
	args = append(args, limit+1)

	rows, err := s.db.Query(
		`SELECT `+ytdlCols+` FROM ytdl_downloads WHERE parent_id = ?`+extra+
			` ORDER BY queue_seq ASC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var out []YtdlDownload
	for rows.Next() {
		d, err := scanYtdlDownload(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var next string
	if len(out) > limit {
		next = encodeYtdlSeqCursor(out[limit-1].QueueSeq)
		out = out[:limit]
	}
	return out, next, nil
}

// YtdlGroupCounts is a group's aggregate, for the one row it occupies in the
// list (FR-019).
type YtdlGroupCounts struct {
	Total, Completed, Failed, Remaining int
}

// YtdlCounts aggregates a group's items.
func (s *Store) YtdlCounts(parentID string) (YtdlGroupCounts, error) {
	var c YtdlGroupCounts
	err := s.db.QueryRow(
		`SELECT COUNT(*),
		        COALESCE(SUM(state = 'completed'), 0),
		        COALESCE(SUM(state = 'failed'), 0)
		   FROM ytdl_downloads WHERE parent_id = ?`, parentID).
		Scan(&c.Total, &c.Completed, &c.Failed)
	c.Remaining = c.Total - c.Completed - c.Failed
	return c, err
}

// SetYtdlState moves a download to a new state.
//
// finished_at is written only on the transition INTO a final state, and never
// cleared here — retry is the one thing allowed to reopen a finished download,
// and it says so explicitly.
func (s *Store) SetYtdlState(requestID, state, reason string, finishedAt *int64) error {
	_, err := s.db.Exec(
		`UPDATE ytdl_downloads SET state = ?, reason = ?, finished_at = COALESCE(?, finished_at)
		 WHERE request_id = ?`, state, reason, finishedAt, requestID)
	return err
}

// SetYtdlCompanion records what the worker produced alongside the media —
// whether a lyrics file was written and in which language (FR-011).
//
// Captured while the worker's output is still readable, because the
// orchestrator sweeps it and this is the only place the fact exists (FR-013g).
func (s *Store) SetYtdlCompanion(requestID string, hasLyrics bool, lang string) error {
	_, err := s.db.Exec(
		`UPDATE ytdl_downloads SET has_lyrics = ?, lyrics_lang = ? WHERE request_id = ?`,
		hasLyrics, lang, requestID)
	return err
}

// YtdlAlreadyHeld reports whether a COMPLETED record exists for this item in
// this mode (FR-020).
//
// Checked at expansion, before anything is queued, so re-running a channel does
// not create rows that would immediately finish having done nothing. A dismissed
// record is gone from this table, so the item counts as not held and is fetched
// again — which is what FR-020a asks for.
func (s *Store) YtdlAlreadyHeld(videoID, mode string) (bool, error) {
	if videoID == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ytdl_downloads
		  WHERE video_id = ? AND mode = ? AND state = 'completed'`, videoID, mode).Scan(&n)
	return n > 0, err
}

// DeleteYtdlDownload removes a download and, when it is a group, its items.
//
// The cascade is the schema's (ON DELETE CASCADE on parent_id), so a group and
// its items can never be half-removed — dismissing is one action to the user and
// must be one action here (FR-005a).
//
// Ownership is in the WHERE clause rather than checked before it, so "not yours"
// and "not there" both come back as no rows affected and the handler cannot
// accidentally tell them apart (FR-008).
func (s *Store) DeleteYtdlDownload(requestID string, userID int64, isAdmin bool) (bool, error) {
	q := `DELETE FROM ytdl_downloads WHERE request_id = ?`
	args := []any{requestID}
	if !isAdmin {
		q += ` AND user_id = ?`
		args = append(args, userID)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

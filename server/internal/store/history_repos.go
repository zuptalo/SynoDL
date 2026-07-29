package store

import (
	"database/sql"
	"strings"
)

// Download-history persistence for the Statistics section (spec 0006). One
// append-only row per downloaded file, written at create time and backfilled
// with the real size by the completion watcher. See migration 0013.

// Download sources. A row's source is set by the writing handler and is never
// client-supplied for catalog sends.
const (
	SourceCatalog = "catalog"
	SourceDirect  = "direct"
)

// Media categories. Catalog sends carry movie/series/anime from the catalog;
// direct downloads are classified (folder + extension) with a user override.
const (
	CategoryMovie      = "movie"
	CategorySeries     = "series"
	CategoryAnime      = "anime"
	CategoryMusicVideo = "music_video"
	CategoryMusic      = "music"
	CategoryOther      = "other"
)

// ValidCategory reports whether s is one of the six known categories. Handlers
// use it to sanitize a client-supplied category (anything else ⇒ fall back to
// the heuristic / "other"); an unknown value must never fail a download.
func ValidCategory(s string) bool {
	switch s {
	case CategoryMovie, CategorySeries, CategoryAnime,
		CategoryMusicVideo, CategoryMusic, CategoryOther:
		return true
	}
	return false
}

// DownloadHistory is one recorded download (write side). CreatedAt is the add
// time; size is unknown until the watcher sees completion.
type DownloadHistory struct {
	UserID      int64
	Source      string // SourceCatalog | SourceDirect
	Category    string
	Destination string
	TaskName    string
	CreatedAt   int64
}

// UserSourceStats is the aggregation for one (user, source) pair. Counts holds
// every row (incl. paused/canceled); AvgSize holds the mean size in bytes over
// COMPLETED rows only, per category plus an "overall" key. A category with no
// completed rows is simply absent from AvgSize (rendered as not-available).
type UserSourceStats struct {
	UserID   int64
	Username string
	Source   string
	Counts   map[string]int   // category -> total count (incl. size-less)
	AvgSize  map[string]int64 // category (and "overall") -> avg bytes, completed only
	// Raw aggregates so a caller can correctly combine across sources/categories
	// (an average of averages would be wrong). Keyed by category.
	Completed map[string]int   // category -> count with a known size
	SumSize   map[string]int64 // category -> sum of known sizes
}

// DayCount is a single day's download count (UTC calendar day of created_at).
type DayCount struct {
	Date  string // "YYYY-MM-DD"
	Count int
}

// AddDownloadHistory records one downloaded file at create time (size unknown).
func (s *Store) AddDownloadHistory(rec DownloadHistory) error {
	_, err := s.db.Exec(
		`INSERT INTO download_history (user_id, source, category, destination, task_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.Source, rec.Category, rec.Destination, rec.TaskName, rec.CreatedAt)
	return err
}

// CompleteDownloadHistory backfills the real size onto the oldest not-yet-
// completed row matching (destination, taskName). Returns false when nothing
// matches (a download not tracked here, or a size already assigned) — the caller
// treats that as a benign no-op, never an error. Matching the OLDEST open row
// keeps multi-episode sends (many rows, one folder) from double-assigning: each
// completing episode fills the next open row.
func (s *Store) CompleteDownloadHistory(destination, taskName string, size, now int64) (bool, error) {
	var id int64
	err := s.db.QueryRow(
		`SELECT id FROM download_history
		 WHERE destination = ? AND task_name = ? AND completed_at IS NULL
		 ORDER BY created_at, id LIMIT 1`, destination, taskName).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := s.db.Exec(
		`UPDATE download_history SET size_bytes = ?, completed_at = ? WHERE id = ?`,
		size, now, id); err != nil {
		return false, err
	}
	return true, nil
}

// inClause builds "?, ?, ?" and the matching args for an IN (...) filter.
func inClause(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ", "), args
}

// StatsSummary aggregates per-category counts and average sizes for the given
// users, split by source. One UserSourceStats per (user, source) that has rows.
func (s *Store) StatsSummary(userIDs []int64) ([]UserSourceStats, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	in, args := inClause(userIDs)
	rows, err := s.db.Query(
		`SELECT h.user_id, u.username, h.source, h.category,
		        COUNT(*)                                                  AS cnt,
		        SUM(h.size_bytes)                                         AS sum_size,
		        SUM(CASE WHEN h.size_bytes IS NOT NULL THEN 1 ELSE 0 END) AS completed
		 FROM download_history h
		 JOIN users u ON u.id = h.user_id
		 WHERE h.user_id IN (`+in+`)
		 GROUP BY h.user_id, h.source, h.category`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Key by (user, source); also track overall sum/completed to derive the
	// overall average correctly (not an average of per-category averages).
	type key struct {
		uid int64
		src string
	}
	acc := map[key]*UserSourceStats{}
	overallSum := map[key]int64{}
	overallCompleted := map[key]int64{}
	order := []key{}

	for rows.Next() {
		var (
			uid            int64
			username, src  string
			category       string
			cnt, completed int64
			sumSize        sql.NullInt64
		)
		if err := rows.Scan(&uid, &username, &src, &category, &cnt, &sumSize, &completed); err != nil {
			return nil, err
		}
		k := key{uid, src}
		st, ok := acc[k]
		if !ok {
			st = &UserSourceStats{
				UserID: uid, Username: username, Source: src,
				Counts: map[string]int{}, AvgSize: map[string]int64{},
				Completed: map[string]int{}, SumSize: map[string]int64{},
			}
			acc[k] = st
			order = append(order, k)
		}
		st.Counts[category] = int(cnt)
		if completed > 0 && sumSize.Valid {
			st.AvgSize[category] = sumSize.Int64 / completed
			st.Completed[category] = int(completed)
			st.SumSize[category] = sumSize.Int64
			overallSum[k] += sumSize.Int64
			overallCompleted[k] += completed
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]UserSourceStats, 0, len(order))
	for _, k := range order {
		st := acc[k]
		if overallCompleted[k] > 0 {
			st.AvgSize["overall"] = overallSum[k] / overallCompleted[k]
		}
		out = append(out, *st)
	}
	return out, nil
}

// StatsDaily returns per-day download counts (UTC calendar day) for the given
// users, optionally filtered to one source ("" = all sources). Only days with
// at least one download are returned; the caller zero-fills the gaps.
func (s *Store) StatsDaily(userIDs []int64, source string) ([]DayCount, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	in, args := inClause(userIDs)
	q := `SELECT date(created_at, 'unixepoch') AS d, COUNT(*)
	      FROM download_history
	      WHERE user_id IN (` + in + `)`
	if source == SourceCatalog || source == SourceDirect {
		q += ` AND source = ?`
		args = append(args, source)
	}
	q += ` GROUP BY d ORDER BY d`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayCount
	for rows.Next() {
		var dc DayCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, err
		}
		out = append(out, dc)
	}
	return out, rows.Err()
}

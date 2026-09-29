package store

import (
	"database/sql"
	"errors"
	"strings"
)

// The music library repair's durable record (spec 1053): what was asked for and how
// it ended. What a run is doing right now is derived from its Job and never
// stored — there is no progress column, on purpose.

const (
	// MusicRepairHistory is how many runs are kept. Older ones are pruned when a
	// new run starts; nothing about the library depends on them.
	MusicRepairHistory = 50
	// MusicRepairSummaryMax bounds the stored summary. The summary is produced by
	// re-serialising a fixed, capped shape, so a real one is a few KiB; this is a
	// backstop that keeps the table from ever holding something log-sized.
	MusicRepairSummaryMax = 16 << 10
)

// ErrRepairBusy means a repair is already running: the single slot is taken.
var ErrRepairBusy = errors.New("a repair is already running")

// MusicRepair is one run's durable record.
type MusicRepair struct {
	ID         string
	Kind       string // check | apply | undo
	PlanID     string
	State      string // running | finished | refused | unfinished
	UserID     *int64 // nil once the account is deleted
	UserName   string // a snapshot, so history survives the user
	StartedAt  int64
	FinishedAt *int64
	Summary    string // JSON of the fixed summary, or ""
}

// StartMusicRepair records a run as running. Inserting IS acquiring the single
// slot: the partial unique index on state='running' makes a second concurrent
// insert fail with ErrRepairBusy.
func (s *Store) StartMusicRepair(r MusicRepair) error {
	_, err := s.db.Exec(
		`INSERT INTO music_repairs (id, kind, plan_id, state, user_id, user_name, started_at, summary)
		 VALUES (?, ?, ?, 'running', ?, ?, ?, '')`,
		r.ID, r.Kind, r.PlanID, r.UserID, r.UserName, r.StartedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "state") {
			return ErrRepairBusy
		}
		return err
	}
	// Keep the newest MusicRepairHistory; never touch the run that is running.
	_, err = s.db.Exec(
		`DELETE FROM music_repairs
		  WHERE state != 'running'
		    AND id NOT IN (SELECT id FROM music_repairs ORDER BY started_at DESC, rowid DESC LIMIT ?)`,
		MusicRepairHistory)
	return err
}

// FinishMusicRepair records how a run ended. It applies only to a run that is
// still running, so a recorded outcome is written once and never rewritten; it
// reports whether it applied.
func (s *Store) FinishMusicRepair(id, state, summary string, finishedAt int64) (bool, error) {
	if len(summary) > MusicRepairSummaryMax {
		summary = ""
	}
	res, err := s.db.Exec(
		`UPDATE music_repairs SET state = ?, summary = ?, finished_at = ? WHERE id = ? AND state = 'running'`,
		state, summary, finishedAt, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const musicRepairCols = `id, kind, plan_id, state, user_id, user_name, started_at, finished_at, summary`

func scanMusicRepair(sc interface{ Scan(...any) error }) (MusicRepair, error) {
	var r MusicRepair
	var uid, fin sql.NullInt64
	if err := sc.Scan(&r.ID, &r.Kind, &r.PlanID, &r.State, &uid, &r.UserName, &r.StartedAt, &fin, &r.Summary); err != nil {
		return r, err
	}
	if uid.Valid {
		v := uid.Int64
		r.UserID = &v
	}
	if fin.Valid {
		v := fin.Int64
		r.FinishedAt = &v
	}
	return r, nil
}

// GetMusicRepair returns one run, or sql.ErrNoRows.
func (s *Store) GetMusicRepair(id string) (*MusicRepair, error) {
	r, err := scanMusicRepair(s.db.QueryRow(`SELECT `+musicRepairCols+` FROM music_repairs WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RunningMusicRepair returns the run holding the slot, or nil.
func (s *Store) RunningMusicRepair() (*MusicRepair, error) {
	r, err := scanMusicRepair(s.db.QueryRow(`SELECT ` + musicRepairCols + ` FROM music_repairs WHERE state = 'running'`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListMusicRepairs returns the newest runs first.
func (s *Store) ListMusicRepairs(limit int) ([]MusicRepair, error) {
	rows, err := s.db.Query(
		`SELECT `+musicRepairCols+` FROM music_repairs ORDER BY started_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MusicRepair
	for rows.Next() {
		r, err := scanMusicRepair(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

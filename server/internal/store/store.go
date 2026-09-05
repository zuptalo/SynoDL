package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (cgo-free; keeps the static alpine build)
)

// Store is the SQLite-backed persistence layer. It owns the *sql.DB and the
// at-rest Cipher; typed repositories hang off it.
type Store struct {
	db     *sql.DB
	cipher *Cipher
}

// Open opens (creating if absent) the SQLite database at dsn and runs pending
// migrations. dsn is a file path (e.g. /data/synodl.db) or ":memory:" in tests.
func Open(dsn string, cipher *Cipher) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// WAL for concurrent readers alongside the single writer; enforce foreign
	// keys (ON DELETE CASCADE); wait rather than fail on a brief write lock.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, cipher: cipher}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle for repositories in this package.
func (s *Store) DB() *sql.DB { return s.db }

// SchemaVersion returns the count of applied migrations.
func (s *Store) SchemaVersion() (int, error) {
	var v int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	current, err := s.SchemaVersion()
	if err != nil {
		return err
	}
	for i := current; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			// A column this migration adds may already be there. That is not a
			// conflict to fail on — it means this installation already has what the
			// migration is for, and refusing to start would strand it.
			//
			// It happens when a migration is added in the wrong place and then
			// corrected: an installation created from the mistaken list has the
			// column at one version, while one upgraded through it does not, and the
			// corrected list has to land on both. Adding a column is idempotent in
			// intent, so it is treated that way.
			if !isDuplicateColumn(err) {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d: %w", i+1, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, i+1); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// isDuplicateColumn reports whether err is SQLite objecting that a column being
// added is already present.
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}

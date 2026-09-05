package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (cgo-free; keeps the static alpine build)
)

// Store is the SQLite-backed persistence layer. It owns the *sql.DB and the
// at-rest Cipher; typed repositories hang off it.
type Store struct {
	db     *sql.DB
	cipher *Cipher
}

// connString turns a file path into a DSN carrying the pragmas every connection
// needs, plus the locking mode write transactions need.
//
// The pragmas used to be applied with a single db.Exec after opening. That is
// wrong in a way that hides well: *sql.DB is a POOL, and busy_timeout and
// foreign_keys are per-CONNECTION settings, so only whichever connection served
// that one Exec ever had them. Every connection the pool opened afterwards ran
// with busy_timeout=0 — meaning it did not wait on a held write lock at all, it
// failed immediately — and with foreign keys OFF, so ON DELETE CASCADE quietly
// did not cascade. (journal_mode=WAL was the exception: it is a property of the
// database file, so it stuck.)
//
// This surfaced when a background writer was added: a scan writing at boot while
// a request wrote too, on two different pooled connections, failed at once
// instead of waiting — and only on a machine whose timing lined up, which is the
// worst kind of bug to own.
//
// _txlock=immediate makes a transaction take the write lock when it BEGINs
// rather than when it first writes. Two deferred transactions that both later
// try to upgrade deadlock, and SQLite returns SQLITE_BUSY for that case
// immediately no matter what busy_timeout says, because waiting cannot resolve
// it. Taking the lock up front turns that unrecoverable case into an ordinary
// wait that busy_timeout does handle.
func connString(dsn string) string {
	// An in-memory database is per-connection by definition; leave its DSN alone
	// rather than imply the pragmas mean anything across the pool.
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Set("_txlock", "immediate")
	return "file:" + dsn + "?" + q.Encode()
}

// Open opens (creating if absent) the SQLite database at dsn and runs pending
// migrations. dsn is a file path (e.g. /data/synodl.db) or ":memory:" in tests.
func Open(dsn string, cipher *Cipher) (*Store, error) {
	db, err := sql.Open("sqlite", connString(dsn))
	if err != nil {
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

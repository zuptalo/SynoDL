package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// openTestStore opens a fresh file-backed store in a temp dir (WAL needs a real
// file, not :memory:, to exercise the production path).
func openTestStore(t *testing.T) *Store {
	t.Helper()
	c, err := NewCipher("kdf-input-for-tests")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	s, err := Open(filepath.Join(t.TempDir(), "synodl.db"), c)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenRunsMigrations(t *testing.T) {
	s := openTestStore(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}
	// Every table from data-model.md must exist.
	for _, tbl := range []string{
		"operator_config", "users", "sessions", "folder_grants",
		"push_subscriptions", "instance", "watched_tasks",
	} {
		var name string
		err := s.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", tbl, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCipher("k")
	dsn := filepath.Join(dir, "synodl.db")
	s1, err := Open(dsn, c)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = s1.Close()
	// Re-opening the same volume must not re-run migrations or error.
	s2, err := Open(dsn, c)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer s2.Close()
	v, _ := s2.SchemaVersion()
	if v != len(migrations) {
		t.Fatalf("after re-open schema version = %d, want %d", v, len(migrations))
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := openTestStore(t)
	// A session referencing a non-existent user must be rejected (FK ON).
	_, err := s.DB().Exec(
		`INSERT INTO sessions(token_hash, user_id, created_at, expires_at) VALUES('h', 999, 0, 0)`)
	if err == nil {
		t.Fatal("expected FK violation inserting a session for a missing user")
	}
}

// Migrations are append-only, and this is the test that says so.
//
// One was once added in the MIDDLE of the list. An installation records how many
// it has applied, so the inserted one sat below that mark and was skipped
// forever — while the code that selected its column shipped anyway, and every
// request for the source list answered 500 on a live instance. The insertion also
// shifted its neighbour down, so installations already past it ran that a second
// time; it happened to be an idempotent UPDATE, which is the only reason nothing
// was corrupted.
//
// The checksums below pin the statements that have already been applied out in
// the world. Adding a migration means appending one line here. Changing this test
// any other way means changing history someone has already run.
func TestMigrationsAreAppendOnly(t *testing.T) {
	want := []string{}
	for _, m := range migrations {
		sum := sha256.Sum256([]byte(m))
		want = append(want, hex.EncodeToString(sum[:8]))
	}
	// Golden: the migrations as they have shipped, in order.
	golden := migrationGolden
	if len(want) < len(golden) {
		t.Fatalf("migrations were REMOVED: %d now, %d before. An installation cannot un-apply one.", len(want), len(golden))
	}
	for i := range golden {
		if want[i] != golden[i] {
			t.Fatalf("migration %d (version %d) changed.\n"+
				"Migrations are append-only: an installation past this version will never run the new text, "+
				"and everything after it shifts. Add a NEW migration at the end instead.", i, i+1)
		}
	}
	if len(want) > len(golden) {
		t.Logf("%d new migration(s) appended — add their checksums to migrationGolden:", len(want)-len(golden))
		for i := len(golden); i < len(want); i++ {
			t.Logf("\t%q, // version %d", want[i], i+1)
		}
		t.Fatal("append the checksums above to migrationGolden")
	}
}

// The pragmas used to be set with one db.Exec after opening. *sql.DB is a POOL
// and busy_timeout / foreign_keys are per-CONNECTION, so only the connection
// that happened to serve that Exec ever had them: every later connection ran
// with no busy timeout (failing instantly on a held write lock instead of
// waiting) and with foreign keys OFF, so ON DELETE CASCADE did not cascade.
//
// This checks several connections at once, because checking one proves nothing —
// that is exactly how the original held up.
func TestEveryPooledConnectionGetsThePragmas(t *testing.T) {
	s := openTestStore(t)

	const conns = 5
	// Hold several connections open simultaneously, forcing the pool to make new
	// ones rather than hand back the single primed connection.
	txs := make([]*sql.Tx, 0, conns)
	for i := 0; i < conns; i++ {
		tx, err := s.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatalf("BeginTx %d: %v", i, err)
		}
		txs = append(txs, tx)
	}
	for i, tx := range txs {
		var busy int
		if err := tx.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
			t.Fatalf("conn %d busy_timeout: %v", i, err)
		}
		if busy == 0 {
			t.Errorf("conn %d has busy_timeout=0: it will fail instantly on a write lock instead of waiting", i)
		}
		var fk int
		if err := tx.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatalf("conn %d foreign_keys: %v", i, err)
		}
		if fk != 1 {
			t.Errorf("conn %d has foreign_keys off: ON DELETE CASCADE will not cascade", i)
		}
	}
	for _, tx := range txs {
		_ = tx.Rollback()
	}
}

// Concurrent writers must serialise rather than fail. This is the shape the
// background library scan introduced: something writing on its own schedule
// while requests write too.
func TestConcurrentWritersDoNotFailEachOther(t *testing.T) {
	s := openTestStore(t)
	const writers = 8

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := s.SaveLibraryEvidence(LibraryEvidence{
					Path:      fmt.Sprintf("movie/W%d-%d", n, j),
					HasVideo:  true,
					CheckedAt: time.Unix(1_700_000_000, 0),
				}); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("a concurrent writer failed instead of waiting: %v", err)
	}
}

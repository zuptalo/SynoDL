package store

import (
	"path/filepath"
	"testing"
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

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// SynoDL is self-hosted, so DATA_DIR is whatever the operator sets. The DSN is
// handed to SQLite's URI parser, which reads '?' as a query, '#' as a fragment
// and '%xx' as an escape — so a path carrying any of them, concatenated
// naively, opens the wrong file or no file at all.
func TestAStoreOpensUnderAwkwardPaths(t *testing.T) {
	for _, dir := range []string{
		"plain",
		"with space",
		"with#hash",
		"with?question",
		"with%percent",
		"with&amp",
	} {
		t.Run(dir, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), dir)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			path := filepath.Join(root, "synodl.db")
			c, _ := NewCipher("kdf-input-for-tests")
			s, err := Open(path, c)
			if err != nil {
				t.Fatalf("Open(%q): %v", path, err)
			}
			defer func() { _ = s.Close() }()

			// The file must exist where we asked for it, not somewhere the URI
			// parser invented.
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("database was not created at %q: %v", path, err)
			}
			// And the pragmas must still have been applied.
			var busy int
			if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
				t.Fatalf("busy_timeout: %v", err)
			}
			if busy == 0 {
				t.Error("busy_timeout was not applied")
			}
		})
	}
}

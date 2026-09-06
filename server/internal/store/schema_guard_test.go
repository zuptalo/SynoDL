package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// applyFirst builds a database as an installation that stopped after n
// migrations would have it: the first n applied, and the counter saying so.
func applyFirst(t *testing.T, path string, n int) {
	t.Helper()
	db, err := sql.Open("sqlite", connString(path))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER NOT NULL)`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := db.Exec(migrations[i]); err != nil && !isDuplicateColumn(err) {
			t.Fatalf("migration %d: %v", i+1, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, i+1); err != nil {
			t.Fatalf("record %d: %v", i+1, err)
		}
	}
}

// Does a database that UPGRADED through this list end up with the same schema as
// one built from it today, whatever version it upgraded from?
//
// Note what this can and cannot see. It replays the CURRENT list, so it cannot
// discover that a migration was inserted mid-list in the past — from here that
// insertion looks like it was always there. Detecting THAT needs either the
// list's history, which the golden checksums hold and
// TestMigrationsAreAppendOnly checks, or a real database, which the drift check
// in Open handles.
//
// What it does catch is a migration that is not safely re-runnable, or that
// depends on state an earlier or later one changes — the ways this list can stop
// converging without anybody editing an old entry. Every stopping point is
// checked because such a migration is usually only wrong from some of them.
func TestEveryUpgradePathReachesTheSameSchema(t *testing.T) {
	want, err := referenceShape()
	if err != nil {
		t.Fatalf("referenceShape: %v", err)
	}
	for n := 0; n <= len(migrations); n++ {
		path := filepath.Join(t.TempDir(), "synodl.db")
		applyFirst(t, path, n)

		// migrate() ONLY — deliberately not Open(), which would repair the drift
		// before this could see it and leave the test passing for the wrong
		// reason. The repair is a safety net for databases already in the wild;
		// this asks whether the LIST is right, which is the thing a reviewer can
		// still fix.
		db, err := sql.Open("sqlite", connString(path))
		if err != nil {
			t.Fatalf("open at version %d: %v", n, err)
		}
		if err := (&Store{db: db}).migrate(); err != nil {
			t.Fatalf("upgrading from version %d: %v", n, err)
		}
		have, err := shapeOf(db)
		if err != nil {
			t.Fatalf("shapeOf after upgrading from %d: %v", n, err)
		}
		_ = db.Close()

		if drift := schemaDrift(have, want); len(drift) > 0 {
			t.Errorf("an installation that stopped at version %d does not reach the current schema:\n  %s",
				n, strings.Join(drift, "\n  "))
		}
	}
}

// The repair must be able to fix the real thing, not just be present. This is
// spec 2017's exact shape: the column gone, the counter past the migration that
// adds it.
func TestSchemaRepairAddsAColumnAMisplacedMigrationSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synodl.db")
	c, _ := NewCipher("kdf-input-for-tests")
	s, err := Open(path, c)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE source_prefs DROP COLUMN hide_owned`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	// Drift is now real, and the counter is at the top so no migration will run.
	have, _ := shapeOf(s.db)
	want, _ := referenceShape()
	if drift := schemaDrift(have, want); len(drift) != 1 || !strings.Contains(drift[0], "source_prefs.hide_owned") {
		t.Fatalf("precondition: expected exactly the one missing column, got %v", drift)
	}

	repaired, err := s.repairSchema()
	if err != nil {
		t.Fatalf("repairSchema: %v", err)
	}
	if len(repaired) != 1 || repaired[0] != "source_prefs.hide_owned" {
		t.Fatalf("repaired = %v, want [source_prefs.hide_owned]", repaired)
	}
	have, _ = shapeOf(s.db)
	if drift := schemaDrift(have, want); len(drift) != 0 {
		t.Errorf("drift remains after repair: %v", drift)
	}
	// And the column actually works.
	uid, err := s.CreateUser("u", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SaveSourceHideOwned(uid, true, 1); err != nil {
		t.Fatalf("the repaired column does not work: %v", err)
	}
	_ = s.Close()
}

// A database that is already correct must not be touched. A repair that fires on
// every start-up would be a write on every boot and a warning nobody could act on.
func TestSchemaRepairIsANoOpOnAHealthyDatabase(t *testing.T) {
	s := openTestStore(t)
	repaired, err := s.repairSchema()
	if err != nil {
		t.Fatalf("repairSchema: %v", err)
	}
	if len(repaired) != 0 {
		t.Errorf("repaired %v on a healthy database", repaired)
	}
}

// A missing TABLE is reported, not silently invented. Creating one implies
// deciding what belongs in it, and a database missing a whole table has a problem
// an automatic repair should not paper over.
func TestSchemaRepairReportsAMissingTableRatherThanCreatingIt(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.db.Exec(`DROP TABLE library_evidence`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	repaired, err := s.repairSchema()
	if err != nil {
		t.Fatalf("repairSchema: %v", err)
	}
	if len(repaired) != 0 {
		t.Errorf("repaired %v; a missing table must not be auto-created", repaired)
	}
	have, _ := shapeOf(s.db)
	want, _ := referenceShape()
	drift := schemaDrift(have, want)
	found := false
	for _, d := range drift {
		if strings.Contains(d, "missing table library_evidence") {
			found = true
		}
	}
	if !found {
		t.Errorf("a missing table must be reported; drift = %v", drift)
	}
}

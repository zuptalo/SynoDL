package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// fkViolations reports what SQLite itself considers a broken reference. Using
// its own checker rather than a hand-written query means the test cannot agree
// with a mistake in the sweep by making the same one.
func fkViolations(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid any
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, table+" -> "+parent)
	}
	return out
}

// rewindTo removes the record of every migration at or after the one whose text
// contains marker, so reopening runs it.
func rewindTo(t *testing.T, db *sql.DB, marker string) {
	t.Helper()
	idx := -1
	for i, m := range migrations {
		if strings.Contains(m, marker) {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("no migration contains %q; this test no longer tests anything", marker)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version > ?`, idx); err != nil {
		t.Fatalf("rewind: %v", err)
	}
}

// Spec 2019: with foreign keys off, deleting a user or a source left its
// dependent rows behind. The sweep removes exactly what the cascades would have.
func TestTheSweepRemovesWhatTheCascadesShouldHave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synodl.db")
	c, err := NewCipher("kdf-input-for-tests")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	s, err := Open(path, c)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	keep, err := s.CreateUser("keep", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	doomed, err := s.CreateUser("doomed", "h", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	liveProv, err := s.CreateProvider(SourceProvider{
		Kind: "faketest", DisplayName: "Live", Enabled: true, State: SourceActive}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	goneProv, err := s.CreateProvider(SourceProvider{
		Kind: "faketest", DisplayName: "Gone", Enabled: true, State: SourceActive}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	for _, p := range []int64{liveProv, goneProv} {
		if err := s.SaveProviderSession(p, SourceSession{
			Fields: map[string]string{"c": "not-a-real-cookie"}, UserAgent: "ua"}, 1); err != nil {
			t.Fatalf("SaveProviderSession: %v", err)
		}
	}
	for _, u := range []int64{keep, doomed} {
		if err := s.SaveSubscription(u, "https://push.example/"+string(rune('a'+u)), "p", "a", true); err != nil {
			t.Fatalf("SaveSubscription: %v", err)
		}
		if err := s.AddDownloadHistory(DownloadHistory{
			UserID: u, Source: SourceCatalog, Category: CategoryMovie,
			Destination: "movie/X", TaskName: "x.mkv", CreatedAt: 1}); err != nil {
			t.Fatalf("AddDownloadHistory: %v", err)
		}
		if err := s.SaveSourceHideOwned(u, true, 1); err != nil {
			t.Fatalf("SaveSourceHideOwned: %v", err)
		}
	}
	if err := s.SaveSourceDownload(SourceDownload{
		Destination: "movie/Orphan", Title: "Orphan", OwnerID: doomed}, 1); err != nil {
		t.Fatalf("SaveSourceDownload: %v", err)
	}

	// Reproduce the bug: delete the parents with foreign keys OFF, exactly as a
	// pooled connection that never got the pragma would have.
	if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("pragma off: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, doomed); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM source_providers WHERE id = ?`, goneProv); err != nil {
		t.Fatalf("delete provider: %v", err)
	}
	if v := fkViolations(t, s.db); len(v) == 0 {
		t.Fatal("precondition: the deletes should have left orphans behind")
	}
	rewindTo(t, s.db, "DELETE FROM source_provider_secrets")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopening runs the sweep.
	s2, err := Open(path, c)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	if v := fkViolations(t, s2.db); len(v) != 0 {
		t.Errorf("orphans remain after the sweep: %v", v)
	}

	// The credentials of the deleted source are gone; the live one's are not.
	var secrets int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM source_provider_secrets`).Scan(&secrets); err != nil {
		t.Fatalf("count secrets: %v", err)
	}
	if secrets != 1 {
		t.Errorf("source_provider_secrets = %d, want 1 (only the live source's)", secrets)
	}

	// Everything belonging to the surviving user is untouched.
	names, err := s2.RecordedNamesFor("movie/X")
	if err != nil {
		t.Fatalf("RecordedNamesFor: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("the surviving user's history has %d rows, want 1", len(names))
	}
	if hide, err := s2.GetSourceHideOwned(keep); err != nil || !hide {
		t.Errorf("the surviving user's prefs were swept: hide=%v err=%v", hide, err)
	}

	// A SET NULL column is nulled, not deleted: the download outlives the account.
	all, err := s2.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	rec, ok := all["movie/Orphan"]
	if !ok {
		t.Fatal("a download was deleted; its owner column should have been nulled instead")
	}
	if rec.OwnerID != 0 {
		t.Errorf("owner = %d, want cleared", rec.OwnerID)
	}
}

// A healthy database must come through the sweep unchanged.
func TestTheSweepTouchesNothingOnAHealthyDatabase(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("u", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SaveSubscription(uid, "https://push.example/one", "p", "a", true); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	if err := s.SaveSourceHideOwned(uid, true, 1); err != nil {
		t.Fatalf("SaveSourceHideOwned: %v", err)
	}

	rewindTo(t, s.db, "DELETE FROM source_provider_secrets")
	for i := range migrations {
		if strings.Contains(migrations[i], "DELETE FROM source_provider_secrets") {
			if _, err := s.db.Exec(migrations[i]); err != nil {
				t.Fatalf("re-running the sweep: %v", err)
			}
		}
	}
	if hide, err := s.GetSourceHideOwned(uid); err != nil || !hide {
		t.Errorf("the sweep removed a live row: hide=%v err=%v", hide, err)
	}
	var subs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, uid).Scan(&subs); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if subs != 1 {
		t.Errorf("subscriptions = %d, want 1", subs)
	}
}

// The migration alone is not enough. On the reporting instance its version was
// recorded while the statements had no effect — the "recorded as applied, never
// ran" shape again — so the sweep is also driven by what SQLite reports as
// broken, which needs no bookkeeping to be right.
func TestOrphansAreSweptEvenWhenTheMigrationDidNotRun(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("doomed", "h", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	pid, err := s.CreateProvider(SourceProvider{
		Kind: "faketest", DisplayName: "Gone", Enabled: true, State: SourceActive}, 1)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if err := s.SaveProviderSession(pid, SourceSession{
		Fields: map[string]string{"c": "not-a-real-cookie"}, UserAgent: "ua"}, 1); err != nil {
		t.Fatalf("SaveProviderSession: %v", err)
	}
	if err := s.SaveSubscription(uid, "https://push.example/x", "p", "a", true); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	if err := s.SaveSourceDownload(SourceDownload{
		Destination: "movie/Orphan", Title: "Orphan", OwnerID: uid}, 1); err != nil {
		t.Fatalf("SaveSourceDownload: %v", err)
	}

	// The bug, reproduced: parents deleted with foreign keys off. The migration
	// counter is left ALONE, so nothing will re-run it — exactly the state the
	// reporting instance was in.
	if _, err := s.db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("pragma off: %v", err)
	}
	for _, q := range []string{
		`DELETE FROM users WHERE id = ?`,
		`DELETE FROM source_providers WHERE id = ?`,
	} {
		id := uid
		if strings.Contains(q, "source_providers") {
			id = pid
		}
		if _, err := s.db.Exec(q, id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if v := fkViolations(t, s.db); len(v) == 0 {
		t.Fatal("precondition: orphans should exist")
	}

	swept, err := s.sweepOrphans()
	if err != nil {
		t.Fatalf("sweepOrphans: %v", err)
	}
	if len(swept) == 0 {
		t.Fatal("nothing was swept")
	}
	if v := fkViolations(t, s.db); len(v) != 0 {
		t.Errorf("violations remain: %v", v)
	}
	// The credentials of the deleted source are gone.
	var secrets int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM source_provider_secrets`).Scan(&secrets); err != nil {
		t.Fatalf("count: %v", err)
	}
	if secrets != 0 {
		t.Errorf("secrets = %d, want 0", secrets)
	}
	// A SET NULL column is cleared, and its row survives.
	all, err := s.SourceDownloads()
	if err != nil {
		t.Fatalf("SourceDownloads: %v", err)
	}
	rec, ok := all["movie/Orphan"]
	if !ok {
		t.Fatal("the download was deleted; its owner should have been cleared instead")
	}
	if rec.OwnerID != 0 {
		t.Errorf("owner = %d, want cleared", rec.OwnerID)
	}
}

// It must be inert on a healthy database — no reports, and above all no deletes.
func TestTheOrphanSweepIsInertWhenNothingIsBroken(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("u", "h", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.SaveSubscription(uid, "https://push.example/one", "p", "a", true); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	swept, err := s.sweepOrphans()
	if err != nil {
		t.Fatalf("sweepOrphans: %v", err)
	}
	if len(swept) != 0 {
		t.Errorf("swept %v on a healthy database", swept)
	}
	var subs int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM push_subscriptions`).Scan(&subs); err != nil {
		t.Fatalf("count: %v", err)
	}
	if subs != 1 {
		t.Errorf("subscriptions = %d, want 1", subs)
	}
}

package store

import (
	"testing"
	"time"
)

func TestOperatorConfigSaveGet(t *testing.T) {
	s := openTestStore(t)

	if has, _ := s.HasOperatorConfig(); has {
		t.Fatal("fresh store should have no operator config")
	}
	if _, err := s.GetOperatorConfig(); err != ErrNotFound {
		t.Fatalf("GetOperatorConfig on empty = %v, want ErrNotFound", err)
	}

	in := OperatorConfig{
		PublicURL: "https://dl.example.com", NASAddress: "nas.local", NASPort: 5001,
		NASTLSVerify: false, NASAccount: "svc", NASPassword: "pw-fixture", NASUses2FA: true,
	}
	if err := s.SaveOperatorConfig(in); err != nil {
		t.Fatalf("SaveOperatorConfig: %v", err)
	}
	if has, _ := s.HasOperatorConfig(); !has {
		t.Fatal("HasOperatorConfig should be true after save")
	}

	// The password must be encrypted at rest — never stored in the clear.
	var enc []byte
	_ = s.DB().QueryRow(`SELECT nas_password_enc FROM operator_config WHERE id=1`).Scan(&enc)
	if len(enc) == 0 || string(enc) == in.NASPassword {
		t.Fatalf("nas password not encrypted at rest (blob=%q)", enc)
	}

	got, err := s.GetOperatorConfig()
	if err != nil {
		t.Fatalf("GetOperatorConfig: %v", err)
	}
	if got.NASPassword != "pw-fixture" || got.NASAccount != "svc" || got.NASPort != 5001 ||
		got.NASTLSVerify != false || got.NASUses2FA != true || got.PublicURL != in.PublicURL {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Upsert: saving again updates in place (still one row).
	in.NASAccount = "svc2"
	if err := s.SaveOperatorConfig(in); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM operator_config`).Scan(&n)
	if n != 1 {
		t.Fatalf("operator_config rows = %d, want 1 (singleton)", n)
	}
}

func TestUsersCRUD(t *testing.T) {
	s := openTestStore(t)

	id, err := s.CreateUser("Alice", "hash-a", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if n, _ := s.CountUsers(); n != 1 {
		t.Fatalf("CountUsers = %d, want 1", n)
	}

	// Case-insensitive username uniqueness.
	if _, err := s.CreateUser("alice", "x", false); err == nil {
		t.Fatal("duplicate username (different case) should fail")
	}

	// Lookup is case-insensitive.
	u, err := s.GetUserByUsername("ALICE")
	if err != nil || u.ID != id || !u.IsAdmin || !u.IsEnabled {
		t.Fatalf("GetUserByUsername = %+v, %v", u, err)
	}

	if err := s.SetUserPassword(id, "hash-b"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	u, _ = s.GetUserByID(id)
	if u.PasswordHash != "hash-b" {
		t.Fatalf("password not updated: %q", u.PasswordHash)
	}

	if _, err := s.GetUserByUsername("nobody"); err != ErrNotFound {
		t.Fatalf("missing user = %v, want ErrNotFound", err)
	}

	if err := s.DeleteUser(id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if n, _ := s.CountUsers(); n != 0 {
		t.Fatalf("CountUsers after delete = %d, want 0", n)
	}
}

func TestSessionsLifecycle(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("bob", "h", false)

	const now = 1000
	if err := s.CreateSession("tok-hash", uid, now+3600); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	u, err := s.UserForSession("tok-hash", now)
	if err != nil || u.ID != uid {
		t.Fatalf("UserForSession = %+v, %v", u, err)
	}

	// Expired session does not resolve.
	if _, err := s.UserForSession("tok-hash", now+7200); err != ErrNotFound {
		t.Fatalf("expired session = %v, want ErrNotFound", err)
	}

	// Disabling the user invalidates sessions immediately.
	if err := s.SetUserEnabled(uid, false); err != nil {
		t.Fatalf("SetUserEnabled: %v", err)
	}
	if _, err := s.UserForSession("tok-hash", now); err != ErrNotFound {
		t.Fatalf("disabled user's session still resolves: %v", err)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("c", "h", false)
	_ = s.CreateSession("live", uid, 5000)
	_ = s.CreateSession("dead", uid, 1000)
	if err := s.DeleteExpiredSessions(2000); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 1 {
		t.Fatalf("sessions after prune = %d, want 1", n)
	}
}

// Remembered faces (spec 0014). A cache, so every property here is about not
// asking a third party twice — and about the table staying a cache rather than
// quietly becoming a record of who looked at what.
func TestPersonPhotoCache(t *testing.T) {
	s := openTestStore(t)

	// Never asked is not the same as asked and found nothing.
	if _, ok := s.GetPersonPhoto("nm0000621"); ok {
		t.Fatal("an unasked person should be a miss")
	}
	if err := s.PutPersonPhoto("nm0000621", "https://img.invalid/a.jpg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := s.GetPersonPhoto("nm0000621")
	if !ok || !got.Found || got.URL != "https://img.invalid/a.jpg" {
		t.Fatalf("round-trip = %+v ok=%v", got, ok)
	}

	// "No photograph of this person" is a real answer and is cached, or every
	// view of every title they appear in asks again.
	if err := s.PutPersonPhoto("nm0000002", ""); err != nil {
		t.Fatalf("put none: %v", err)
	}
	got, ok = s.GetPersonPhoto("nm0000002")
	if !ok {
		t.Fatal("a cached 'none' should be a hit")
	}
	if got.Found || got.URL != "" {
		t.Fatalf("none came back as %+v", got)
	}
}

// A stale row reads as a miss, so the caller re-resolves and overwrites it. That
// is the whole of expiry — there is no sweeper to go wrong.
func TestPersonPhotoExpiry(t *testing.T) {
	s := openTestStore(t)
	if err := s.PutPersonPhoto("nm0000003", "https://img.invalid/b.jpg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	age := func(id string, d time.Duration) {
		if _, err := s.db.Exec(`UPDATE person_photos SET checked_at = ? WHERE imdb_id = ?`,
			time.Now().Add(-d).Unix(), id); err != nil {
			t.Fatalf("age: %v", err)
		}
	}
	age("nm0000003", personPhotoTTL/2)
	if _, ok := s.GetPersonPhoto("nm0000003"); !ok {
		t.Fatal("a photograph within its life should still be a hit")
	}
	age("nm0000003", personPhotoTTL+time.Hour)
	if _, ok := s.GetPersonPhoto("nm0000003"); ok {
		t.Fatal("an expired photograph should read as a miss")
	}

	// A "none" expires much sooner, because "none" is also what a block looks
	// like and a blocked instance must recover on its own.
	if err := s.PutPersonPhoto("nm0000004", ""); err != nil {
		t.Fatalf("put none: %v", err)
	}
	age("nm0000004", personMissingTTL+time.Hour)
	if _, ok := s.GetPersonPhoto("nm0000004"); ok {
		t.Fatal("an expired 'none' should read as a miss")
	}
	if personMissingTTL >= personPhotoTTL {
		t.Fatal("a missing photograph must be re-checked sooner than a found one")
	}
}

// A source's own handle for somebody resolves once, to both halves.
func TestSourcePersonCache(t *testing.T) {
	s := openTestStore(t)
	want := SourcePerson{IMDbID: "nm0000621", PhotoURL: "https://site.invalid/p.jpg", Name: "Kurt Russell"}
	if err := s.PutSourcePerson("zarfilm", "actor/kurt-russell", want); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := s.GetSourcePerson("zarfilm", "actor/kurt-russell")
	if !ok || got != want {
		t.Fatalf("round-trip = %+v ok=%v", got, ok)
	}
	// Scoped per source: two sources' handles never collide.
	if _, ok := s.GetSourcePerson("30nama", "actor/kurt-russell"); ok {
		t.Fatal("a handle leaked across sources")
	}
}

// The tables are bounded, so a long-running instance cannot fill the operator's
// volume with faces.
func TestPrunePeopleBoundsGrowth(t *testing.T) {
	s := openTestStore(t)
	// One row past its life, and one still fresh.
	if err := s.PutPersonPhoto("nm0000005", "https://img.invalid/c.jpg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE person_photos SET checked_at = ? WHERE imdb_id = ?`,
		time.Now().Add(-2*personPhotoTTL).Unix(), "nm0000005"); err != nil {
		t.Fatalf("age: %v", err)
	}
	if err := s.PutPersonPhoto("nm0000006", "https://img.invalid/d.jpg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	s.PrunePeople()

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM person_photos`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("after pruning: %d rows, want only the fresh one", n)
	}
}

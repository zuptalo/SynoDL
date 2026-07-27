package store

import "testing"

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

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// adminAfterSetup completes the wizard and returns the admin auth header.
func adminAfterSetup(t *testing.T, h http.Handler) map[string]string {
	t.Helper()
	rec := do(t, h, "POST", "/v1/setup", setupBody, nil)
	if rec.Code != 200 {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	return map[string]string{"X-SynoDL-Session": login.Token}
}

func TestGetNASConfigNeverLeaksPassword(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	rec := do(t, h, "GET", "/v1/nas/config", "", admin)
	if rec.Code != 200 {
		t.Fatalf("get config = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The non-secret projection must carry the connection fields...
	for _, want := range []string{`"publicUrl"`, `"nasAddress"`, `"nasPort"`, `"nasAccount"`, `"nasUses2FA"`} {
		if !strings.Contains(body, want) {
			t.Errorf("config missing %s: %s", want, body)
		}
	}
	// ...but NEVER the password, in any form.
	if strings.Contains(strings.ToLower(body), "password") || strings.Contains(body, dsmMockValue) {
		t.Fatalf("config must not expose the NAS password: %s", body)
	}
}

func TestNASConfigEndpointsRequireAdmin(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// No token → 401.
	if rec := do(t, h, "GET", "/v1/nas/config", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token get = %d, want 401", rec.Code)
	}

	// A non-admin user → 403 on every NAS-config route.
	do(t, h, "POST", "/v1/users", `{"username":"bob","password":"example-bob-pw","isAdmin":false}`, admin)
	rec := do(t, h, "POST", "/v1/session", `{"username":"bob","password":"example-bob-pw"}`, nil)
	var bob struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bob)
	bobAuth := map[string]string{"X-SynoDL-Session": bob.Token}

	for _, m := range []struct{ method, path, body string }{
		{"GET", "/v1/nas/config", ""},
		{"PUT", "/v1/nas/config", `{"publicUrl":"https://x"}`},
		{"POST", "/v1/nas/test", `{"nasAddress":"nas","nasPort":5001,"nasAccount":"admin","nasPassword":"` + dsmMockValue + `"}`},
	} {
		if rec := do(t, h, m.method, m.path, m.body, bobAuth); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin = %d, want 403", m.method, m.path, rec.Code)
		}
	}
}

func TestTestNASConnection(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Correct credentials against the mock DSM → 204, nothing persisted.
	ok := `{"nasAddress":"nas","nasPort":5001,"nasTlsVerify":false,"nasAccount":"admin","nasPassword":"` + dsmMockValue + `"}`
	if rec := do(t, h, "POST", "/v1/nas/test", ok, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("test ok = %d %s", rec.Code, rec.Body.String())
	}

	// Wrong password → a credentials error surfaced as 401.
	bad := `{"nasAddress":"nas","nasPort":5001,"nasTlsVerify":false,"nasAccount":"admin","nasPassword":"wrong"}`
	if rec := do(t, h, "POST", "/v1/nas/test", bad, admin); rec.Code != http.StatusUnauthorized {
		t.Fatalf("test wrong password = %d, want 401", rec.Code)
	}

	// Blank password falls back to the stored one (test the existing connection).
	if rec := do(t, h, "POST", "/v1/nas/test", `{"nasAddress":"nas","nasPort":5001,"nasAccount":"admin"}`, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("test with stored password = %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateNASConfigPublicURLOnly(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Editing only the public URL must not re-verify the NAS (works even on a
	// 2FA setup with no OTP to hand).
	if rec := do(t, h, "PUT", "/v1/nas/config", `{"publicUrl":"https://new.example.com"}`, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("update publicUrl = %d %s", rec.Code, rec.Body.String())
	}
	rec := do(t, h, "GET", "/v1/nas/config", "", admin)
	if !strings.Contains(rec.Body.String(), "https://new.example.com") {
		t.Fatalf("publicUrl not updated: %s", rec.Body.String())
	}
}

func TestUpdateNASConfigBadCredentialsRollsBack(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Change the stored password to a wrong one: the manager can't establish a
	// session, so the edit must roll back to the previous working config.
	bad := `{"nasAccount":"admin","nasPassword":"definitely-wrong"}`
	if rec := do(t, h, "PUT", "/v1/nas/config", bad, admin); rec.Code == http.StatusNoContent {
		t.Fatalf("update with bad credentials unexpectedly succeeded")
	}
	// The stored connection still works afterwards (rolled back, not broken).
	if rec := do(t, h, "POST", "/v1/nas/test", `{"nasAddress":"nas","nasPort":5001,"nasAccount":"admin"}`, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("post-rollback test = %d %s, want the old creds to still work", rec.Code, rec.Body.String())
	}
}

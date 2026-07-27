package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestNotifPrefsDefaultsThenUpdate(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Defaults: completions + failures for my own tasks; added off; scope own.
	rec := do(t, h, "GET", "/v1/notifications/prefs", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get prefs = %d %s", rec.Code, rec.Body.String())
	}
	var p notifPrefsView
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.NotifyAdded || !p.NotifyCompleted || !p.NotifyFailed || p.Scope != "own" {
		t.Fatalf("default prefs = %+v", p)
	}

	// Update and read back.
	body := `{"notifyAdded":true,"notifyCompleted":false,"notifyFailed":true,"scope":"any"}`
	if rec := do(t, h, "PUT", "/v1/notifications/prefs", body, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("put prefs = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "GET", "/v1/notifications/prefs", "", admin)
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if !p.NotifyAdded || p.NotifyCompleted || !p.NotifyFailed || p.Scope != "any" {
		t.Fatalf("updated prefs = %+v", p)
	}

	// An unknown scope is coerced to "own".
	do(t, h, "PUT", "/v1/notifications/prefs", `{"scope":"bogus"}`, admin)
	rec = do(t, h, "GET", "/v1/notifications/prefs", "", admin)
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Scope != "own" {
		t.Fatalf("bogus scope should coerce to own, got %q", p.Scope)
	}

	// No token → 401.
	if rec := do(t, h, "GET", "/v1/notifications/prefs", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token get = %d, want 401", rec.Code)
	}
}

func TestTitleHint(t *testing.T) {
	cases := map[string]string{
		"http://mirror.example/path/ubuntu.iso": "ubuntu.iso",
		"https://x/y/":                          "y",
		"magnet:?xt=urn:btih:abc&dn=My%20Movie": "My Movie",
		"magnet:?xt=urn:btih:abc":               "magnet download",
		"ftp://h/file.zip":                      "file.zip",
	}
	for in, want := range cases {
		if got := titleHint(in); got != want {
			t.Errorf("titleHint(%q) = %q, want %q", in, got, want)
		}
	}
}

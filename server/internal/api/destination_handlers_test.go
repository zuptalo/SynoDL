package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func getDestPrefs(t *testing.T, h http.Handler, auth map[string]string) destPrefsView {
	t.Helper()
	rec := do(t, h, "GET", "/v1/destinations/prefs", "", auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("get dest prefs = %d %s", rec.Code, rec.Body.String())
	}
	var p destPrefsView
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p
}

func TestDestinationPrefsPersistAndCleanup(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// Valid prefs persist and come back verbatim (the mock has these folders).
	do(t, h, "PUT", "/v1/destinations/prefs",
		`{"default":"movie","favorites":["movie/4K","tv-show","music"]}`, admin)
	p := getDestPrefs(t, h, admin)
	if p.Default != "movie" || len(p.Favorites) != 3 {
		t.Fatalf("prefs did not persist: %+v", p)
	}

	// A default that no longer exists resets to root; a favorite whose folder is
	// gone (parent still lists, leaf missing) is dropped; an existing one stays.
	do(t, h, "PUT", "/v1/destinations/prefs",
		`{"default":"ghost","favorites":["movie","movie/Gone"]}`, admin)
	p = getDestPrefs(t, h, admin)
	if p.Default != "" {
		t.Fatalf("gone default should reset to root, got %q", p.Default)
	}
	if len(p.Favorites) != 1 || p.Favorites[0] != "movie" {
		t.Fatalf("a missing favorite should be dropped, got %+v", p.Favorites)
	}
}

func TestDestinationPrefsDropOnRevokedAccess(t *testing.T) {
	h, _ := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	rec := do(t, h, "POST", "/v1/users", `{"username":"bob","password":"example-bob-pw","isAdmin":false}`, admin)
	var bob struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bob)
	do(t, h, "PUT", "/v1/users/"+itoa(bob.ID)+"/folders", `{"folders":["movie"]}`, admin)
	rec = do(t, h, "POST", "/v1/session", `{"username":"bob","password":"example-bob-pw"}`, nil)
	var bl struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bl)
	bobAuth := map[string]string{"X-SynoDL-Session": bl.Token}

	// While granted movie, bob makes it his default + favorite.
	do(t, h, "PUT", "/v1/destinations/prefs", `{"default":"movie","favorites":["movie"]}`, bobAuth)
	if p := getDestPrefs(t, h, bobAuth); p.Default != "movie" || len(p.Favorites) != 1 {
		t.Fatalf("bob's prefs = %+v", p)
	}

	// Admin revokes bob's access to movie → bob's next read drops it entirely.
	do(t, h, "PUT", "/v1/users/"+itoa(bob.ID)+"/folders", `{"folders":[]}`, admin)
	p := getDestPrefs(t, h, bobAuth)
	if p.Default != "" || len(p.Favorites) != 0 {
		t.Fatalf("revoked access should drop the default (to root) and favorite, got %+v", p)
	}
}

func TestDestinationPrefsRequireAuth(t *testing.T) {
	h, _ := newStatefulRouter(t)
	if rec := do(t, h, "GET", "/v1/destinations/prefs", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token get = %d, want 401", rec.Code)
	}
}

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The owner is the first user created (the setup admin). Another admin must not
// be able to lock them out, and the owner can never be demoted, disabled, or
// deleted — so the instance always keeps a full-access account.
func TestOwnerIsProtectedFromOtherAdmins(t *testing.T) {
	h, _ := newStatefulRouter(t)
	setup := do(t, h, "POST", "/v1/setup", setupBody, nil) // owner "kamran"
	var owner struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(setup.Body.Bytes(), &owner)
	ownerAuth := map[string]string{"X-SynoDL-Session": owner.Token}

	// The owner creates a second admin.
	rec := do(t, h, "POST", "/v1/users", `{"username":"carol","password":"example-carol-pw","isAdmin":true}`, ownerAuth)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create admin = %d, body %s", rec.Code, rec.Body.String())
	}
	var carol struct {
		ID      int64 `json:"id"`
		IsOwner bool  `json:"isOwner"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &carol)
	if carol.IsOwner {
		t.Fatal("a newly created user must not be flagged as owner")
	}

	// The listing flags the owner, and only the owner.
	rec = do(t, h, "GET", "/v1/users", "", ownerAuth)
	var list struct {
		Users []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			IsOwner  bool   `json:"isOwner"`
		} `json:"users"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	var ownerID int64
	owners := 0
	for _, u := range list.Users {
		if u.IsOwner {
			owners++
			ownerID = u.ID
			if u.Username != "kamran" {
				t.Fatalf("owner flag on wrong user %q", u.Username)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("want exactly one owner, got %d", owners)
	}

	// Carol signs in as the second admin.
	rec = do(t, h, "POST", "/v1/session", `{"username":"carol","password":"example-carol-pw"}`, nil)
	var carolLogin struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &carolLogin)
	carolAuth := map[string]string{"X-SynoDL-Session": carolLogin.Token}

	ownerPath := "/v1/users/" + itoa(ownerID)

	// Carol (another admin) cannot touch the owner in any way → 403.
	for _, body := range []string{
		`{"isAdmin":false}`,
		`{"isEnabled":false}`,
		`{"password":"a-new-owner-password"}`,
		`{"contentRating":"R"}`,
	} {
		if rec := do(t, h, "PATCH", ownerPath, body, carolAuth); rec.Code != http.StatusForbidden {
			t.Fatalf("carol PATCH owner %s = %d, want 403", body, rec.Code)
		}
	}
	if rec := do(t, h, "DELETE", ownerPath, "", carolAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("carol DELETE owner = %d, want 403", rec.Code)
	}

	// Even the owner cannot demote, disable, or delete themselves — the account
	// must always stay a full-access admin.
	if rec := do(t, h, "PATCH", ownerPath, `{"isAdmin":false}`, ownerAuth); rec.Code != http.StatusBadRequest {
		t.Fatalf("owner self-demote = %d, want 400", rec.Code)
	}
	if rec := do(t, h, "PATCH", ownerPath, `{"isEnabled":false}`, ownerAuth); rec.Code != http.StatusBadRequest {
		t.Fatalf("owner self-disable = %d, want 400", rec.Code)
	}
	if rec := do(t, h, "DELETE", ownerPath, "", ownerAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("owner self-delete = %d, want 403", rec.Code)
	}

	// The owner can still manage other admins — here, demote and re-enable carol.
	carolPath := "/v1/users/" + itoa(carol.ID)
	if rec := do(t, h, "PATCH", carolPath, `{"isAdmin":false}`, ownerAuth); rec.Code != http.StatusOK {
		t.Fatalf("owner demotes carol = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, "PATCH", carolPath, `{"password":"another-strong-pw"}`, ownerAuth); rec.Code != http.StatusOK {
		t.Fatalf("owner resets carol = %d", rec.Code)
	}
	if rec := do(t, h, "DELETE", carolPath, "", ownerAuth); rec.Code != http.StatusNoContent {
		t.Fatalf("owner deletes carol = %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/v1/users", "", ownerAuth); strings.Contains(rec.Body.String(), `"carol"`) {
		t.Fatalf("carol still listed after delete: %s", rec.Body.String())
	}
}

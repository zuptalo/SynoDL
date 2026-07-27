package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"synodl/server/internal/syno"
)

func TestCreateFolderStatelessForwardsAndReturns(t *testing.T) {
	fake := &fakeSyno{}
	srv := newTestServer(t, fake)
	resp := doReq(t, srv, http.MethodPost, "/v1/fs/folder", "sid", "application/json",
		strings.NewReader(`{"path":"/movie","name":"Docs"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if fake.gotFolderPath != "/movie" || fake.gotFolderName != "Docs" {
		t.Errorf("forwarded = %q/%q", fake.gotFolderPath, fake.gotFolderName)
	}
	var out struct {
		Folder syno.Folder `json:"folder"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Folder.Path != "/movie/Docs" {
		t.Errorf("folder = %+v", out.Folder)
	}
}

func TestCreateFolderRejectsBadInput(t *testing.T) {
	srv := newTestServer(t, &fakeSyno{})
	for _, b := range []string{
		`{"path":"movie","name":"x"}`,      // parent not absolute
		`{"path":"/movie","name":""}`,      // empty name
		`{"path":"/movie","name":".."}`,    // dot entry
		`{"path":"/movie","name":"a/b"}`,   // path separator in name
		`{"path":"/movie","name":"..\\e"}`, // backslash separator
	} {
		resp := doReq(t, srv, http.MethodPost, "/v1/fs/folder", "sid", "application/json", strings.NewReader(b))
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s → %d, want 400", b, resp.StatusCode)
		}
	}
}

func TestCreateFolderStatefulEnforcesCreateACL(t *testing.T) {
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

	// Bob may create inside his granted "movie" share...
	if rec := do(t, h, "POST", "/v1/fs/folder", `{"path":"/movie","name":"Docs"}`, bobAuth); rec.Code != http.StatusOK {
		t.Fatalf("bob create in /movie = %d %s", rec.Code, rec.Body.String())
	}
	// ...but not under an ungranted share.
	if rec := do(t, h, "POST", "/v1/fs/folder", `{"path":"/home","name":"x"}`, bobAuth); rec.Code != http.StatusForbidden {
		t.Fatalf("bob create in /home = %d, want 403", rec.Code)
	}
	// Admin may create anywhere.
	if rec := do(t, h, "POST", "/v1/fs/folder", `{"path":"/home","name":"stuff"}`, admin); rec.Code != http.StatusOK {
		t.Fatalf("admin create in /home = %d %s", rec.Code, rec.Body.String())
	}
	// No token → 401.
	if rec := do(t, h, "POST", "/v1/fs/folder", `{"path":"/movie","name":"y"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token create = %d, want 401", rec.Code)
	}
}

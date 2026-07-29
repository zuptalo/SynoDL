package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The quota endpoint reports unlimited by default, and an admin can reset a
// user's daily count.
func TestSourceQuotaAndAdminReset(t *testing.T) {
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	// A fresh admin has no cap → unlimited (remaining -1).
	rec := do(t, h, "GET", "/v1/source/quota", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("quota = %d %s", rec.Code, rec.Body.String())
	}
	var q struct{ Limit, Used, Remaining int }
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if q.Limit != 0 || q.Remaining != -1 {
		t.Fatalf("default quota = %+v, want unlimited", q)
	}

	// Create a capped user (limit 3) and burn 2 downloads.
	bobID, err := st.CreateUser("bob", "h", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := st.SetUserDailyDownloadLimit(bobID, 3); err != nil {
		t.Fatalf("SetUserDailyDownloadLimit: %v", err)
	}
	_ = st.AddDownloadEvents(bobID, 2, 9_000_000_000) // far-future stamp stays in-window

	// The admin listing shows bob's used count.
	rec = do(t, h, "GET", "/v1/users", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users = %d", rec.Code)
	}
	var list struct {
		Users []struct {
			ID            int64 `json:"id"`
			DownloadsUsed int   `json:"downloadsUsed"`
		} `json:"users"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, u := range list.Users {
		if u.ID == bobID {
			found = true
			if u.DownloadsUsed != 2 {
				t.Fatalf("bob downloadsUsed = %d, want 2", u.DownloadsUsed)
			}
		}
	}
	if !found {
		t.Fatal("bob missing from user list")
	}

	// Admin resets bob's count → back to 0.
	path := "/v1/users/" + itoa(bobID) + "/downloads/reset"
	if rec := do(t, h, "POST", path, "", admin); rec.Code != http.StatusNoContent {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body.String())
	}
	if n, _ := st.CountUserDownloadsSince(bobID, 0); n != 0 {
		t.Fatalf("count after reset = %d, want 0", n)
	}

	// Reset of an unknown user is a clean 404.
	if rec := do(t, h, "POST", "/v1/users/99999/downloads/reset", "", admin); rec.Code != http.StatusNotFound {
		t.Fatalf("reset unknown = %d, want 404", rec.Code)
	}

	// The reset endpoint is admin-only — an anonymous request is rejected.
	if rec := do(t, h, "POST", path, "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token reset = %d, want 401", rec.Code)
	}
}

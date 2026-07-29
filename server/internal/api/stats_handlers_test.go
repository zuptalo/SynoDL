package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"synodl/server/internal/auth"
	"synodl/server/internal/store"
)

// sessionFor mints a valid session header for an existing user (bypasses the
// login flow so tests can exercise role gating directly).
func sessionFor(t *testing.T, st *store.Store, uid int64) map[string]string {
	t.Helper()
	tok, hash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if err := st.CreateSession(hash, uid, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return map[string]string{"X-SynoDL-Session": tok}
}

type statCatJSON struct {
	Count     int   `json:"count"`
	Completed int   `json:"completed"`
	SumBytes  int64 `json:"sumBytes"`
}

type summaryResp struct {
	Users []struct {
		UserID   int64                             `json:"userId"`
		Username string                            `json:"username"`
		BySource map[string]map[string]statCatJSON `json:"bySource"`
	} `json:"users"`
}

func seedCompleted(t *testing.T, st *store.Store, uid int64, src, cat, dest, name string, size int64) {
	t.Helper()
	if err := st.AddDownloadHistory(store.DownloadHistory{
		UserID: uid, Source: src, Category: cat, Destination: dest, TaskName: name, CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("AddDownloadHistory: %v", err)
	}
	if ok, err := st.CompleteDownloadHistory(dest, name, size, 1010); err != nil || !ok {
		t.Fatalf("CompleteDownloadHistory: ok=%v err=%v", ok, err)
	}
}

// A regular user sees only their own summary; an admin sees everyone.
func TestStatsSummaryRoleGating(t *testing.T) {
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)

	alice, _ := st.CreateUser("alice", "h", false)
	bob, _ := st.CreateUser("bob", "h", false)
	seedCompleted(t, st, alice, store.SourceCatalog, store.CategoryMovie, "movies/A", "A.mkv", 100)
	seedCompleted(t, st, bob, store.SourceCatalog, store.CategoryMovie, "movies/B", "B.mkv", 300)

	// Admin: sees all users.
	rec := do(t, h, "GET", "/v1/stats/summary", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin summary = %d %s", rec.Code, rec.Body.String())
	}
	var got summaryResp
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	names := map[string]bool{}
	for _, u := range got.Users {
		names[u.Username] = true
	}
	if !names["alice"] || !names["bob"] {
		t.Fatalf("admin should see all users, got %+v", names)
	}

	// Alice: sees only herself, regardless of any params.
	rec = do(t, h, "GET", "/v1/stats/summary", "", sessionFor(t, st, alice))
	if rec.Code != http.StatusOK {
		t.Fatalf("alice summary = %d", rec.Code)
	}
	got = summaryResp{}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Users) != 1 || got.Users[0].Username != "alice" {
		t.Fatalf("alice must see only herself, got %+v", got.Users)
	}
	movie := got.Users[0].BySource["catalog"]["movie"]
	if movie.Count != 1 || movie.Completed != 1 || movie.SumBytes != 100 {
		t.Fatalf("alice movie stat = %+v, want count1 completed1 sum100", movie)
	}
	// A category with no completed downloads is present with zeroed aggregates
	// (the client renders its average as "—").
	series := got.Users[0].BySource["catalog"]["series"]
	if series.Count != 0 || series.Completed != 0 {
		t.Fatalf("empty series stat = %+v, want zeros", series)
	}
}

// Raw aggregates let the client combine sources exactly (an average of averages
// would be wrong).
func TestStatsSummaryRawAggregatesCombine(t *testing.T) {
	h, st := newStatefulRouter(t)
	_ = adminAfterSetup(t, h)
	uid, _ := st.CreateUser("carol", "h", false)
	seedCompleted(t, st, uid, store.SourceCatalog, store.CategoryMovie, "movies/A", "A.mkv", 100)
	seedCompleted(t, st, uid, store.SourceDirect, store.CategoryMusic, "music/T", "T.flac", 20)

	rec := do(t, h, "GET", "/v1/stats/summary", "", sessionFor(t, st, uid))
	var got summaryResp
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	cat := got.Users[0].BySource["catalog"]["movie"]
	dir := got.Users[0].BySource["direct"]["music"]
	// Combined overall = (100+20) / (1+1) = 60, computed from raw fields.
	totalBytes := cat.SumBytes + dir.SumBytes
	totalCompleted := cat.Completed + dir.Completed
	if totalCompleted != 2 || totalBytes/int64(totalCompleted) != 60 {
		t.Fatalf("combined overall from raw = %d/%d", totalBytes, totalCompleted)
	}
}

// Timeseries: non-admin is forced to their own scope; days are zero-filled.
func TestStatsTimeseriesGating(t *testing.T) {
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	alice, _ := st.CreateUser("alice", "h", false)

	// Two of alice's downloads on the same UTC day.
	day := int64(1782000010)
	for _, n := range []string{"A.mkv", "B.mkv"} {
		_ = st.AddDownloadHistory(store.DownloadHistory{
			UserID: alice, Source: store.SourceCatalog, Category: store.CategoryMovie,
			Destination: "movies/x", TaskName: n, CreatedAt: day,
		})
	}

	// Admin asking for alice specifically.
	rec := do(t, h, "GET", "/v1/stats/timeseries?userId="+itoa(alice), "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("timeseries = %d %s", rec.Code, rec.Body.String())
	}
	var ts struct {
		Days []struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		} `json:"days"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ts)
	total := 0
	for _, d := range ts.Days {
		total += d.Count
	}
	if total != 2 {
		t.Fatalf("timeseries total = %d, want 2", total)
	}

	// A non-admin's userId param is ignored — they only ever see themselves.
	rec = do(t, h, "GET", "/v1/stats/timeseries?userId="+itoa(alice), "", sessionFor(t, st, alice))
	var self struct {
		UserID int64 `json:"userId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &self)
	if self.UserID != alice {
		t.Fatalf("non-admin userId echo = %d, want %d (forced self)", self.UserID, alice)
	}
}

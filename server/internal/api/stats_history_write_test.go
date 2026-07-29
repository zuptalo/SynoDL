package api

import (
	"net/http"
	"testing"

	"synodl/server/internal/store"
)

// catalogCountFor returns the total catalog download-history count across all
// users (test convenience).
func historyCounts(t *testing.T, st *store.Store) []store.UserSourceStats {
	t.Helper()
	users, _ := st.ListUsers()
	ids := make([]int64, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	sum, err := st.StatsSummary(ids)
	if err != nil {
		t.Fatalf("StatsSummary: %v", err)
	}
	return sum
}

// A catalog send records one catalog history row per file, categorized by the
// send's type.
func TestSourceSendRecordsCatalogHistory(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	fakeLinks = []string{"http://dl.fake/soul.mkv"}

	rec := do(t, h, "POST", "/v1/source/send",
		`{"titleId":"217561","qualityId":"q1","title":"Soul 2020","type":"movie"}`, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("send = %d %s", rec.Code, rec.Body.String())
	}
	sum := historyCounts(t, st)
	var catalogMovies int
	for _, s := range sum {
		if s.Source == store.SourceCatalog {
			catalogMovies += s.Counts[store.CategoryMovie]
		}
	}
	if catalogMovies != 1 {
		t.Fatalf("catalog movie history rows = %d, want 1", catalogMovies)
	}
}

// A direct add records a direct history row attributed to the adder; an explicit
// category wins, and the daily limit is NOT charged for direct downloads.
func TestDirectAddRecordsDirectHistory(t *testing.T) {
	h, st := newStatefulRouter(t)
	do(t, h, "POST", "/v1/setup", setupBody, nil)
	users, _ := st.ListUsers()
	adminID := users[0].ID
	admin := sessionFor(t, st, adminID)

	// Explicit category "series" overrides the folder heuristic (movies/…).
	body := `{"uris":["magnet:?xt=urn:btih:deadbeef&dn=Cool.Show.S01E01.mkv"],` +
		`"destination":"movies/Cool","category":"series"}`
	if rec := do(t, h, "POST", "/v1/tasks", body, admin); rec.Code != http.StatusCreated {
		t.Fatalf("direct add = %d %s", rec.Code, rec.Body.String())
	}

	sum := historyCounts(t, st)
	if len(sum) != 1 || sum[0].Source != store.SourceDirect || sum[0].Counts[store.CategorySeries] != 1 {
		t.Fatalf("direct history = %+v, want one direct series row", sum)
	}
	// Direct downloads must not count against the daily limit (catalog-only).
	if n, _ := st.CountUserDownloadsSince(adminID, 0); n != 0 {
		t.Fatalf("direct add charged the daily limit: used=%d, want 0", n)
	}
}

// With no explicit category, a direct add is classified from folder + file type.
func TestDirectAddHeuristicCategory(t *testing.T) {
	h, st := newStatefulRouter(t)
	do(t, h, "POST", "/v1/setup", setupBody, nil)
	users, _ := st.ListUsers()
	admin := sessionFor(t, st, users[0].ID)

	// Audio file, no category → classified as music.
	body := `{"uris":["magnet:?xt=urn:btih:abc&dn=track.flac"],"destination":"Downloads"}`
	if rec := do(t, h, "POST", "/v1/tasks", body, admin); rec.Code != http.StatusCreated {
		t.Fatalf("direct add = %d %s", rec.Code, rec.Body.String())
	}
	sum := historyCounts(t, st)
	if len(sum) != 1 || sum[0].Counts[store.CategoryMusic] != 1 {
		t.Fatalf("heuristic category = %+v, want one music row", sum)
	}
}

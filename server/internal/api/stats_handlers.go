package api

import (
	"net/http"
	"strconv"
	"time"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
)

// Statistics endpoints (spec 0006). Read-only aggregates over download_history.
// Visibility is role-gated SERVER-SIDE: a non-admin only ever sees their own
// data (a crafted request can't widen scope); an admin/owner sees every user.

// jsonCategory maps a stored category to its client-facing key (the client uses
// camelCase musicVideo; everything else is identical).
func jsonCategory(cat string) string {
	if cat == store.CategoryMusicVideo {
		return "musicVideo"
	}
	return cat
}

// the fixed set of category buckets echoed for every source (present even when
// zero, so the client renders a stable grid).
var categoryOrder = []string{
	store.CategoryMovie, store.CategorySeries, store.CategoryAnime,
	store.CategoryMusicVideo, store.CategoryMusic, store.CategoryOther,
}

// statCat carries RAW aggregates (not a pre-computed average): the client can
// then combine categories, sources, and users exactly — averaging averages
// would be wrong — and render avg = sumBytes/completed (or "—" when completed=0).
type statCat struct {
	Count     int   `json:"count"`     // all rows, incl. paused/canceled
	Completed int   `json:"completed"` // rows with a known size
	SumBytes  int64 `json:"sumBytes"`  // sum of known sizes
}

type statUserSummary struct {
	UserID   int64                         `json:"userId"`
	Username string                        `json:"username"`
	BySource map[string]map[string]statCat `json:"bySource"` // "catalog"|"direct" -> category -> stat
}

// emptySource is a zero-filled per-category grid for one source.
func emptySource() map[string]statCat {
	m := make(map[string]statCat, len(categoryOrder))
	for _, cat := range categoryOrder {
		m[jsonCategory(cat)] = statCat{}
	}
	return m
}

// visibleUsers returns the users whose stats the caller may see: everyone for an
// admin/owner, just themselves for a regular user.
func (d Deps) visibleUsers(u *store.User) ([]store.User, error) {
	if u.IsAdmin {
		return d.Store.ListUsers()
	}
	return []store.User{*u}, nil
}

func userIDs(users []store.User) []int64 {
	ids := make([]int64, len(users))
	for i, us := range users {
		ids[i] = us.ID
	}
	return ids
}

// handleGetStatsSummary returns per-user, per-source category counts + average
// sizes (catalog, direct, and combined "all").
func handleGetStatsSummary(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, _ *http.Request, u *store.User) {
		users, err := d.visibleUsers(u)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		rows, err := d.Store.StatsSummary(userIDs(users))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		// Every visible user gets a row with both sources zero-filled across all
		// categories, so the client renders a stable grid; then overlay the data.
		byUser := map[int64]*statUserSummary{}
		out := make([]statUserSummary, 0, len(users))
		for _, us := range users {
			out = append(out, statUserSummary{
				UserID: us.ID, Username: us.Username,
				BySource: map[string]map[string]statCat{
					store.SourceCatalog: emptySource(),
					store.SourceDirect:  emptySource(),
				},
			})
			byUser[us.ID] = &out[len(out)-1]
		}
		for _, r := range rows {
			su := byUser[r.UserID]
			if su == nil {
				continue
			}
			src := su.BySource[r.Source]
			if src == nil {
				continue // unknown source spelling — ignore defensively
			}
			for _, cat := range categoryOrder {
				if c := r.Counts[cat]; c != 0 {
					src[jsonCategory(cat)] = statCat{Count: c, Completed: r.Completed[cat], SumBytes: r.SumSize[cat]}
				}
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"users": out})
	})
}

// handleGetStatsTimeseries returns zero-filled daily download counts for the
// caller's visible scope, optionally filtered to one source and (admins only)
// one user.
func handleGetStatsTimeseries(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		source := r.URL.Query().Get("source") // "", catalog, direct, all
		ids, who := d.timeseriesScope(u, r.URL.Query().Get("userId"))
		days, err := d.Store.StatsDaily(ids, source)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"userId": who, "source": source, "days": zeroFillDays(days),
		})
	})
}

// timeseriesScope resolves which users the series covers and the echoed userId.
// A non-admin is always forced to themselves. An admin may pass a specific
// userId, or "all"/empty for everyone.
func (d Deps) timeseriesScope(u *store.User, userIDParam string) (ids []int64, who any) {
	if !u.IsAdmin {
		return []int64{u.ID}, u.ID
	}
	if userIDParam == "" || userIDParam == "all" {
		if users, err := d.Store.ListUsers(); err == nil {
			return userIDs(users), "all"
		}
		return nil, "all"
	}
	if id, err := strconv.ParseInt(userIDParam, 10, 64); err == nil {
		return []int64{id}, id
	}
	return []int64{u.ID}, u.ID
}

// zeroFillDays turns the sparse per-day rows into a contiguous daily series from
// the first recorded day to today, so gaps render as zero rather than vanish.
func zeroFillDays(days []store.DayCount) []map[string]any {
	if len(days) == 0 {
		return []map[string]any{}
	}
	const layout = "2006-01-02"
	start, err := time.Parse(layout, days[0].Date)
	if err != nil {
		// Fall back to the raw rows if the first date can't be parsed.
		out := make([]map[string]any, len(days))
		for i, dc := range days {
			out[i] = map[string]any{"date": dc.Date, "count": dc.Count}
		}
		return out
	}
	counts := make(map[string]int, len(days))
	for _, dc := range days {
		counts[dc.Date] = dc.Count
	}
	end := time.Now().UTC().Truncate(24 * time.Hour)
	out := []map[string]any{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		key := day.Format(layout)
		out = append(out, map[string]any{"date": key, "count": counts[key]})
	}
	return out
}

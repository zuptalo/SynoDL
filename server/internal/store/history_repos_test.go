package store

import "testing"

// helper: one completed download (add then backfill size).
func addCompleted(t *testing.T, s *Store, uid int64, src, cat, dest, name string, size, when int64) {
	t.Helper()
	if err := s.AddDownloadHistory(DownloadHistory{
		UserID: uid, Source: src, Category: cat, Destination: dest, TaskName: name, CreatedAt: when,
	}); err != nil {
		t.Fatalf("AddDownloadHistory: %v", err)
	}
	ok, err := s.CompleteDownloadHistory(dest, name, size, when+10)
	if err != nil || !ok {
		t.Fatalf("CompleteDownloadHistory(%s/%s) ok=%v err=%v", dest, name, ok, err)
	}
}

func TestDownloadHistorySummary(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("alice", "h", false)

	// Two completed movies (sizes 100, 300 -> avg 200), one series completed (50),
	// and one series still in flight (size-less: counts, but no size sample).
	addCompleted(t, s, uid, SourceCatalog, CategoryMovie, "movies/A", "A.mkv", 100, 1000)
	addCompleted(t, s, uid, SourceCatalog, CategoryMovie, "movies/B", "B.mkv", 300, 1000)
	addCompleted(t, s, uid, SourceCatalog, CategorySeries, "tv/S", "S.E01.mkv", 50, 1000)
	if err := s.AddDownloadHistory(DownloadHistory{
		UserID: uid, Source: SourceCatalog, Category: CategorySeries,
		Destination: "tv/S", TaskName: "S.E02.mkv", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("AddDownloadHistory (in-flight): %v", err)
	}

	sum, err := s.StatsSummary([]int64{uid})
	if err != nil {
		t.Fatalf("StatsSummary: %v", err)
	}
	if len(sum) != 1 {
		t.Fatalf("want 1 (user,source) row, got %d", len(sum))
	}
	st := sum[0]
	if st.Username != "alice" || st.Source != SourceCatalog {
		t.Fatalf("unexpected row: %+v", st)
	}
	// Counts include the in-flight series episode.
	if st.Counts[CategoryMovie] != 2 || st.Counts[CategorySeries] != 2 {
		t.Fatalf("counts = %+v, want movie:2 series:2", st.Counts)
	}
	// Averages over completed rows only.
	if st.AvgSize[CategoryMovie] != 200 {
		t.Fatalf("avg movie = %d, want 200", st.AvgSize[CategoryMovie])
	}
	if st.AvgSize[CategorySeries] != 50 {
		t.Fatalf("avg series = %d, want 50 (in-flight excluded)", st.AvgSize[CategorySeries])
	}
	// Overall = (100+300+50)/3 = 150 (three completed, size-less excluded).
	if st.AvgSize["overall"] != 150 {
		t.Fatalf("overall avg = %d, want 150", st.AvgSize["overall"])
	}
}

func TestDownloadHistorySummaryNoCompletedIsAbsent(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("bob", "h", false)
	// A single in-flight (size-less) direct movie: counted, but no size sample.
	if err := s.AddDownloadHistory(DownloadHistory{
		UserID: uid, Source: SourceDirect, Category: CategoryMovie,
		Destination: "movies/X", TaskName: "X.mkv", CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("AddDownloadHistory: %v", err)
	}
	sum, _ := s.StatsSummary([]int64{uid})
	if len(sum) != 1 || sum[0].Counts[CategoryMovie] != 1 {
		t.Fatalf("want movie count 1, got %+v", sum)
	}
	if _, ok := sum[0].AvgSize[CategoryMovie]; ok {
		t.Fatalf("avg movie should be absent (no completed rows), got %d", sum[0].AvgSize[CategoryMovie])
	}
	if _, ok := sum[0].AvgSize["overall"]; ok {
		t.Fatalf("overall avg should be absent when nothing completed")
	}
}

func TestDownloadHistorySourceSplit(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("carol", "h", false)
	addCompleted(t, s, uid, SourceCatalog, CategoryMovie, "movies/A", "A.mkv", 100, 1000)
	addCompleted(t, s, uid, SourceDirect, CategoryMusic, "music/T", "T.flac", 10, 1000)

	sum, _ := s.StatsSummary([]int64{uid})
	if len(sum) != 2 {
		t.Fatalf("want 2 (user,source) rows, got %d", len(sum))
	}
	bySrc := map[string]UserSourceStats{}
	for _, st := range sum {
		bySrc[st.Source] = st
	}
	if bySrc[SourceCatalog].Counts[CategoryMovie] != 1 || bySrc[SourceDirect].Counts[CategoryMusic] != 1 {
		t.Fatalf("source split wrong: %+v", bySrc)
	}
}

func TestCompleteDownloadHistoryMatchingAndNoMatch(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("dave", "h", false)

	// Two episodes in one folder, distinct names.
	for _, n := range []string{"S.E01.mkv", "S.E02.mkv"} {
		if err := s.AddDownloadHistory(DownloadHistory{
			UserID: uid, Source: SourceCatalog, Category: CategorySeries,
			Destination: "tv/S", TaskName: n, CreatedAt: 1000,
		}); err != nil {
			t.Fatalf("add %s: %v", n, err)
		}
	}
	// First completion fills E01's row; a second call for the same pair must NOT
	// re-fill the same row (already completed) — it finds nothing.
	if ok, _ := s.CompleteDownloadHistory("tv/S", "S.E01.mkv", 500, 2000); !ok {
		t.Fatal("first E01 completion should match")
	}
	if ok, _ := s.CompleteDownloadHistory("tv/S", "S.E01.mkv", 999, 2000); ok {
		t.Fatal("second E01 completion should NOT match (already completed)")
	}
	// A completion for a download we never recorded is a benign no-match.
	if ok, _ := s.CompleteDownloadHistory("tv/Unknown", "nope.mkv", 1, 2000); ok {
		t.Fatal("unknown completion should not match")
	}

	sum, _ := s.StatsSummary([]int64{uid})
	// Two series rows total; only one has a size (avg = 500).
	if sum[0].Counts[CategorySeries] != 2 {
		t.Fatalf("series count = %d, want 2", sum[0].Counts[CategorySeries])
	}
	if sum[0].AvgSize[CategorySeries] != 500 {
		t.Fatalf("series avg = %d, want 500", sum[0].AvgSize[CategorySeries])
	}
}

func TestStatsDailyGroupsByDay(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("erin", "h", false)
	// 2026-07-01 00:00:10 UTC = 1782000010 ; same day another; next day one.
	day1 := int64(1782000010)
	day2 := day1 + 86400
	for _, ts := range []int64{day1, day1 + 60, day2} {
		_ = s.AddDownloadHistory(DownloadHistory{
			UserID: uid, Source: SourceCatalog, Category: CategoryMovie,
			Destination: "movies/A", TaskName: "A.mkv", CreatedAt: ts,
		})
	}
	days, err := s.StatsDaily([]int64{uid}, "")
	if err != nil {
		t.Fatalf("StatsDaily: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("want 2 distinct days, got %d (%+v)", len(days), days)
	}
	if days[0].Count != 2 || days[1].Count != 1 {
		t.Fatalf("day counts = %+v, want [2,1]", days)
	}
	// Source filter narrows results.
	direct, _ := s.StatsDaily([]int64{uid}, SourceDirect)
	if len(direct) != 0 {
		t.Fatalf("direct-only should be empty, got %+v", direct)
	}
}

func TestDownloadHistoryCascadesWithUser(t *testing.T) {
	s := openTestStore(t)
	uid, _ := s.CreateUser("frank", "h", false)
	addCompleted(t, s, uid, SourceCatalog, CategoryMovie, "movies/A", "A.mkv", 100, 1000)

	if err := s.DeleteUser(uid); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM download_history WHERE user_id = ?`, uid).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("history rows after user delete = %d, want 0 (cascade)", n)
	}
}

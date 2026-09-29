package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func run(id, kind string, uid *int64) MusicRepair {
	return MusicRepair{ID: id, Kind: kind, State: "running", UserID: uid, UserName: "Anna", StartedAt: 1_790_000_000}
}

func TestMusicRepair_TheSlotIsHeldByTheDatabase(t *testing.T) {
	s := openTestStore(t)
	if err := s.StartMusicRepair(run("aaaaaaaaaaaa", "check", nil)); err != nil {
		t.Fatalf("first start: %v", err)
	}
	err := s.StartMusicRepair(run("bbbbbbbbbbbb", "check", nil))
	if !errors.Is(err, ErrRepairBusy) {
		t.Fatalf("second start: %v, want ErrRepairBusy", err)
	}
	if _, gone := s.GetMusicRepair("bbbbbbbbbbbb"); gone == nil {
		t.Error("a refused request left a row behind")
	}
	if ok, err := s.FinishMusicRepair("aaaaaaaaaaaa", "finished", "{}", 1_790_000_100); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}
	if err := s.StartMusicRepair(run("cccccccccccc", "apply", nil)); err != nil {
		t.Fatalf("start after the slot was freed: %v", err)
	}
}

func TestMusicRepair_SimultaneousStartsYieldExactlyOne(t *testing.T) {
	s := openTestStore(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.StartMusicRepair(run(fmt.Sprintf("%012x", i+1), "check", nil))
			if err == nil {
				mu.Lock()
				won++
				mu.Unlock()
			} else if !errors.Is(err, ErrRepairBusy) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("%d requests started, want exactly 1 (SC-003)", won)
	}
}

func TestMusicRepair_FinishIsOnceOnly(t *testing.T) {
	s := openTestStore(t)
	_ = s.StartMusicRepair(run("aaaaaaaaaaaa", "check", nil))
	if ok, _ := s.FinishMusicRepair("aaaaaaaaaaaa", "finished", `{"kind":"check"}`, 1_790_000_100); !ok {
		t.Fatal("first finish did not apply")
	}
	if ok, _ := s.FinishMusicRepair("aaaaaaaaaaaa", "unfinished", "", 1_790_000_999); ok {
		t.Error("a second finish overwrote a recorded outcome")
	}
	got, err := s.GetMusicRepair("aaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "finished" || got.Summary != `{"kind":"check"}` || got.FinishedAt == nil || *got.FinishedAt != 1_790_000_100 {
		t.Errorf("row = %+v", got)
	}
}

func TestMusicRepair_ListIsNewestFirstAndBounded(t *testing.T) {
	s := openTestStore(t)
	for i := 1; i <= 5; i++ {
		r := run(fmt.Sprintf("%012x", i), "check", nil)
		r.StartedAt = int64(1_790_000_000 + i*10)
		if err := s.StartMusicRepair(r); err != nil {
			t.Fatal(err)
		}
		_, _ = s.FinishMusicRepair(r.ID, "finished", "", r.StartedAt+5)
	}
	got, err := s.ListMusicRepairs(3)
	if err != nil || len(got) != 3 {
		t.Fatalf("list = %d %v", len(got), err)
	}
	if got[0].ID != fmt.Sprintf("%012x", 5) || got[2].ID != fmt.Sprintf("%012x", 3) {
		t.Errorf("order = %s, %s, %s: want newest first", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestMusicRepair_OnlyTheNewest50AreKept_NeverTheRunningOne(t *testing.T) {
	s := openTestStore(t)
	for i := 1; i <= 60; i++ {
		r := run(fmt.Sprintf("%012x", i), "check", nil)
		r.StartedAt = int64(1_790_000_000 + i)
		if err := s.StartMusicRepair(r); err != nil {
			t.Fatal(err)
		}
		_, _ = s.FinishMusicRepair(r.ID, "finished", "", r.StartedAt+1)
	}
	all, _ := s.ListMusicRepairs(1000)
	if len(all) != MusicRepairHistory {
		t.Fatalf("rows = %d, want the newest %d", len(all), MusicRepairHistory)
	}
	if all[len(all)-1].ID != fmt.Sprintf("%012x", 11) {
		t.Errorf("oldest kept = %s, want the 11th run", all[len(all)-1].ID)
	}
	// A running row is never pruned, however old.
	old := run("ffffffffffff", "check", nil)
	old.StartedAt = 1
	if err := s.StartMusicRepair(old); err != nil {
		t.Fatal(err)
	}
	if r, err := s.RunningMusicRepair(); err != nil || r == nil || r.ID != "ffffffffffff" {
		t.Errorf("running = %+v %v", r, err)
	}
}

func TestMusicRepair_TheHistoryOutlivesTheUser(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", true)
	_ = s.StartMusicRepair(run("aaaaaaaaaaaa", "apply", &anna))
	_, _ = s.FinishMusicRepair("aaaaaaaaaaaa", "finished", "", 1_790_000_100)
	if _, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, anna); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	got, err := s.GetMusicRepair("aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("the run vanished with its starter: %v", err)
	}
	if got.UserID != nil || got.UserName != "Anna" {
		t.Errorf("row = %+v, want the name kept and the id cleared", got)
	}
}

func TestMusicRepair_AnOversizeSummaryIsNotStored(t *testing.T) {
	s := openTestStore(t)
	_ = s.StartMusicRepair(run("aaaaaaaaaaaa", "check", nil))
	if _, err := s.FinishMusicRepair("aaaaaaaaaaaa", "finished", strings.Repeat("x", MusicRepairSummaryMax+1), 1_790_000_100); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetMusicRepair("aaaaaaaaaaaa")
	if got.Summary != "" {
		t.Errorf("summary length = %d, want it dropped: the store holds a bounded summary, not a log", len(got.Summary))
	}
}

func TestMusicRepair_TheMigrationCanBeReplayed(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.db.Exec(migrations[len(migrations)-1]); err != nil {
		t.Fatalf("replaying the newest migration: %v (the drift repair replays them, so each must be idempotent)", err)
	}
}

func TestMusicRepair_ARunThatNeverStartedCanBeDiscarded_ButAnOutcomeCannot(t *testing.T) {
	s := openTestStore(t)
	_ = s.StartMusicRepair(run("aaaaaaaaaaaa", "check", nil))
	if ok, err := s.DiscardMusicRepair("aaaaaaaaaaaa"); err != nil || !ok {
		t.Fatalf("discard: %v %v", ok, err)
	}
	if r, _ := s.RunningMusicRepair(); r != nil {
		t.Error("the slot is still held")
	}
	_ = s.StartMusicRepair(run("bbbbbbbbbbbb", "check", nil))
	_, _ = s.FinishMusicRepair("bbbbbbbbbbbb", "finished", "{}", 1_790_000_100)
	if ok, _ := s.DiscardMusicRepair("bbbbbbbbbbbb"); ok {
		t.Error("a recorded outcome was erased")
	}
}

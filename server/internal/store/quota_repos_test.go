package store

import "testing"

func TestDownloadEventsCountAndReset(t *testing.T) {
	s := openTestStore(t)
	uid, err := s.CreateUser("dl", "h", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// No downloads yet.
	if n, err := s.CountUserDownloadsSince(uid, 0); err != nil || n != 0 {
		t.Fatalf("empty count = %d, %v", n, err)
	}

	// Five downloads at t=1000; a stale one at t=100.
	if err := s.AddDownloadEvents(uid, 5, 1000); err != nil {
		t.Fatalf("AddDownloadEvents: %v", err)
	}
	_ = s.AddDownloadEvents(uid, 1, 100)

	// The rolling window excludes the stale event.
	if n, _ := s.CountUserDownloadsSince(uid, 500); n != 5 {
		t.Fatalf("windowed count = %d, want 5", n)
	}
	// A wide-open window counts everything.
	if n, _ := s.CountUserDownloadsSince(uid, 0); n != 6 {
		t.Fatalf("full count = %d, want 6", n)
	}

	// Adding zero (or negative) is a no-op, not an error.
	if err := s.AddDownloadEvents(uid, 0, 2000); err != nil {
		t.Fatalf("AddDownloadEvents(0): %v", err)
	}

	// Reset clears the log — fresh allowance.
	if err := s.ResetUserDownloads(uid); err != nil {
		t.Fatalf("ResetUserDownloads: %v", err)
	}
	if n, _ := s.CountUserDownloadsSince(uid, 0); n != 0 {
		t.Fatalf("count after reset = %d, want 0", n)
	}

	// One user's reset doesn't touch another's count.
	other, _ := s.CreateUser("other", "h", false)
	_ = s.AddDownloadEvents(other, 3, 1000)
	_ = s.ResetUserDownloads(uid)
	if n, _ := s.CountUserDownloadsSince(other, 0); n != 3 {
		t.Fatalf("other user's count affected by reset: %d", n)
	}
}

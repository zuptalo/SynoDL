package store

import "testing"

func failure(id string) YtdlFailure {
	return YtdlFailure{
		RequestID: id,
		SourceURL: "https://youtu.be/" + id,
		Mode:      "music",
		Scope:     "single",
		Reason:    "the download did not complete",
		FailedAt:  1757000000,
	}
}

func TestRecordAndListYtdlFailures(t *testing.T) {
	s := openTestStore(t)

	if err := s.RecordYtdlFailure(failure("aaa")); err != nil {
		t.Fatalf("record: %v", err)
	}
	f2 := failure("bbb")
	f2.FailedAt = 1757000100 // newer
	if err := s.RecordYtdlFailure(f2); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := s.ListYtdlFailures()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 failures, got %d", len(got))
	}
	if got[0].RequestID != "bbb" {
		t.Errorf("newest must come first, got %q", got[0].RequestID)
	}
	if got[0].SourceURL != "https://youtu.be/bbb" || got[0].Mode != "music" {
		t.Errorf("row not round-tripped: %+v", got[0])
	}
}

// State is polled, so the same failed job is observed again on every refresh.
// Re-observing must not duplicate the row, and must not keep moving the
// timestamp — the first sighting is when it failed.
func TestRecordYtdlFailure_IsIdempotent(t *testing.T) {
	s := openTestStore(t)

	first := failure("dup")
	if err := s.RecordYtdlFailure(first); err != nil {
		t.Fatal(err)
	}
	later := failure("dup")
	later.FailedAt = first.FailedAt + 5000
	later.Reason = "a different reason"
	if err := s.RecordYtdlFailure(later); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListYtdlFailures()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("re-observing a failure must not duplicate it: got %d rows", len(got))
	}
	if got[0].FailedAt != first.FailedAt {
		t.Errorf("failed_at moved on re-observation: %d, want %d", got[0].FailedAt, first.FailedAt)
	}
}

func TestDeleteYtdlFailure(t *testing.T) {
	s := openTestStore(t)
	if err := s.RecordYtdlFailure(failure("gone")); err != nil {
		t.Fatal(err)
	}

	ok, err := s.DeleteYtdlFailure("gone")
	if err != nil || !ok {
		t.Fatalf("delete = %v, %v; want true, nil", ok, err)
	}
	got, _ := s.ListYtdlFailures()
	if len(got) != 0 {
		t.Errorf("row survived deletion: %+v", got)
	}

	ok, err = s.DeleteYtdlFailure("never-existed")
	if err != nil {
		t.Fatalf("deleting an unknown id should not error: %v", err)
	}
	if ok {
		t.Error("deleting an unknown id should report false")
	}
}

// FR-024a: a failure log is a courtesy, not an audit trail. On a home NAS an
// unbounded one is a slow leak.
func TestRecordYtdlFailure_IsBounded(t *testing.T) {
	s := openTestStore(t)
	for i := 0; i < maxYtdlFailures+25; i++ {
		f := failure(string(rune('a'+i%26)) + string(rune('a'+i/26)) + itoa(i))
		f.FailedAt = int64(1757000000 + i)
		if err := s.RecordYtdlFailure(f); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListYtdlFailures()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxYtdlFailures {
		t.Fatalf("table grew to %d rows, want it capped at %d", len(got), maxYtdlFailures)
	}
	// The rows kept must be the NEWEST ones.
	if got[0].FailedAt != int64(1757000000+maxYtdlFailures+24) {
		t.Errorf("pruning kept the wrong rows: newest is %d", got[0].FailedAt)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// Ownership-scoped queries (spec 0013). Filtering in SQL is the point: a row the
// caller may not see must never enter the process.

func TestListYtdlFailuresFor_ScopesToTheOwner(t *testing.T) {
	s := openTestStore(t)
	one, _ := s.CreateUser("anna", "h", false)
	two, _ := s.CreateUser("bo", "h", false)

	mine := failure("mine")
	mine.UserID = &one
	theirs := failure("theirs")
	theirs.UserID = &two
	orphan := failure("orphan") // user_id NULL: the account was deleted

	for _, f := range []YtdlFailure{mine, theirs, orphan} {
		if err := s.RecordYtdlFailure(f); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := s.ListYtdlFailuresFor(one, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != "mine" {
		t.Fatalf("user 1 sees %+v, want only their own row", got)
	}

	all, err := s.ListYtdlFailuresFor(one, true)
	if err != nil {
		t.Fatalf("list as admin: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("admin sees %d rows, want all three including the unattributed one", len(all))
	}
}

func TestDeleteYtdlFailureFor_NotYoursIsIndistinguishableFromNotThere(t *testing.T) {
	s := openTestStore(t)
	one, _ := s.CreateUser("anna", "h", false)
	two, _ := s.CreateUser("bo", "h", false)

	theirs := failure("theirs")
	theirs.UserID = &two
	if err := s.RecordYtdlFailure(theirs); err != nil {
		t.Fatalf("record: %v", err)
	}

	notYours, err := s.DeleteYtdlFailureFor("theirs", one, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	notThere, err := s.DeleteYtdlFailureFor("never-existed", one, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if notYours || notThere {
		t.Fatalf("removed = (%v, %v), want both false so the two cases cannot be told apart", notYours, notThere)
	}
	// And the row is still there — a refused dismissal must not delete anything.
	if got, _ := s.ListYtdlFailuresFor(two, false); len(got) != 1 {
		t.Fatalf("owner now sees %d rows, want their row untouched", len(got))
	}

	// An admin may dismiss it.
	ok, err := s.DeleteYtdlFailureFor("theirs", one, true)
	if err != nil || !ok {
		t.Fatalf("admin delete = (%v, %v), want it removed", ok, err)
	}
}

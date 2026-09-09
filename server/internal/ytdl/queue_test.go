package ytdl

import "testing"

func ids(cs []Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.RequestID)
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAdmit_SharesSlotsBetweenUsers(t *testing.T) {
	// The case this rule exists for: Anna expands a big channel, then Bo pastes
	// one song. Strict first-in-first-out would make Bo wait for the channel.
	var queued []Candidate
	for i := 0; i < 500; i++ {
		queued = append(queued, Candidate{RequestID: "anna-item", UserID: 1, Direct: false, Seq: int64(i)})
	}
	queued = append(queued, Candidate{RequestID: "bo-song", UserID: 2, Direct: true, Seq: 900})

	got := ids(Admit(queued, 4))
	var sawBo bool
	for _, id := range got {
		if id == "bo-song" {
			sawBo = true
		}
	}
	if !sawBo {
		t.Fatalf("admitted %v — Bo's single link must not wait behind 500 of Anna's", got)
	}
}

func TestAdmit_PastedLinkBeatsYourOwnBulkWork(t *testing.T) {
	// FR-022c. Anna's own song should not be stuck behind Anna's own channel.
	queued := []Candidate{
		{RequestID: "channel-1", UserID: 1, Direct: false, Seq: 1},
		{RequestID: "channel-2", UserID: 1, Direct: false, Seq: 2},
		{RequestID: "pasted", UserID: 1, Direct: true, Seq: 99}, // queued LAST
	}
	got := ids(Admit(queued, 1))
	if !eq(got, "pasted") {
		t.Fatalf("admitted %v, want the pasted link first even though it was queued last", got)
	}
}

func TestAdmit_OneUserGetsEverySlot(t *testing.T) {
	// Fair-share must not idle a slot. The limit is a ceiling, not a per-user
	// reservation — with only one user queued, they get all of it.
	var queued []Candidate
	for i := 0; i < 10; i++ {
		queued = append(queued, Candidate{RequestID: "only", UserID: 1, Seq: int64(i)})
	}
	if got := Admit(queued, 4); len(got) != 4 {
		t.Fatalf("admitted %d of 4 slots for a sole user, want all of them", len(got))
	}
}

func TestAdmit_RotatesBetweenUsers(t *testing.T) {
	queued := []Candidate{
		{RequestID: "a1", UserID: 1, Seq: 1},
		{RequestID: "a2", UserID: 1, Seq: 2},
		{RequestID: "a3", UserID: 1, Seq: 3},
		{RequestID: "b1", UserID: 2, Seq: 4},
		{RequestID: "b2", UserID: 2, Seq: 5},
	}
	got := ids(Admit(queued, 4))
	if !eq(got, "a1", "b1", "a2", "b2") {
		t.Fatalf("admitted %v, want the slots to alternate between the two users", got)
	}
}

func TestAdmit_SubmissionOrderIsTheTiebreak(t *testing.T) {
	queued := []Candidate{
		{RequestID: "third", UserID: 1, Direct: true, Seq: 3},
		{RequestID: "first", UserID: 1, Direct: true, Seq: 1},
		{RequestID: "second", UserID: 1, Direct: true, Seq: 2},
	}
	if got := ids(Admit(queued, 3)); !eq(got, "first", "second", "third") {
		t.Fatalf("admitted %v, want submission order among equals", got)
	}
}

func TestAdmit_NeverExceedsTheLimit(t *testing.T) {
	var queued []Candidate
	for i := 0; i < 50; i++ {
		queued = append(queued, Candidate{RequestID: "x", UserID: int64(i%5 + 1), Seq: int64(i)})
	}
	for _, slots := range []int{1, 2, 4, 7} {
		if got := Admit(queued, slots); len(got) != slots {
			t.Errorf("slots=%d admitted %d", slots, len(got))
		}
	}
}

func TestAdmit_NothingToDo(t *testing.T) {
	if got := Admit(nil, 4); len(got) != 0 {
		t.Errorf("empty queue admitted %v", got)
	}
	if got := Admit([]Candidate{{RequestID: "a", UserID: 1}}, 0); len(got) != 0 {
		t.Errorf("no free slots admitted %v", got)
	}
	// FR-006d: a deleted account's queued work never starts. The record
	// survives as history; nobody is waiting for the download.
	orphan := []Candidate{{RequestID: "orphan", UserID: 0, Seq: 1}}
	if got := Admit(orphan, 4); len(got) != 0 {
		t.Errorf("admitted %v for a deleted account", ids(got))
	}
}

func TestAdmit_IsDeterministic(t *testing.T) {
	// Admission runs every cycle; if it depended on map iteration order the
	// queue would shuffle between ticks and nobody could reason about it.
	queued := []Candidate{
		{RequestID: "a1", UserID: 1, Seq: 1},
		{RequestID: "b1", UserID: 2, Seq: 2},
		{RequestID: "c1", UserID: 3, Seq: 3},
		{RequestID: "a2", UserID: 1, Seq: 4},
	}
	first := ids(Admit(queued, 3))
	for i := 0; i < 50; i++ {
		if got := ids(Admit(queued, 3)); !eq(got, first...) {
			t.Fatalf("run %d admitted %v, first run admitted %v", i, got, first)
		}
	}
}

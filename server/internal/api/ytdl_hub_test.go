package api

import (
	"encoding/json"
	"testing"
	"time"
)

func chg(id string, owner *int64) ytdlChange {
	return ytdlChange{
		OwnerID:   owner,
		RequestID: id,
		Admin:     json.RawMessage(`{"requestId":"` + id + `","submittedBy":"anna"}`),
		Plain:     json.RawMessage(`{"requestId":"` + id + `"}`),
	}
}

// recv takes one batch, or fails: a hub that silently delivers nothing would
// pass every other assertion in this file.
func recv(t *testing.T, ch <-chan []ytdlChange) []ytdlChange {
	t.Helper()
	select {
	case b, ok := <-ch:
		if !ok {
			t.Fatal("channel closed, want a batch")
		}
		return b
	case <-time.After(time.Second):
		t.Fatal("no batch delivered")
		return nil
	}
}

func TestYtdlHub_DeliversToASubscriber(t *testing.T) {
	h := newYtdlHub()
	anna := int64(1)
	ch, cancel := h.subscribe(anna, false)
	defer cancel()

	h.publish([]ytdlChange{chg("a", &anna)})

	got := recv(t, ch)
	if len(got) != 1 || got[0].RequestID != "a" {
		t.Fatalf("got %+v, want the one change published", got)
	}
}

// The stream must not show what a poll would not (FR-006). Three rules, one
// test, because they are one rule with three cases.
func TestYtdlHub_CarriesOnlyWhatAReaderMaySee(t *testing.T) {
	anna, bo := int64(1), int64(2)

	h := newYtdlHub()
	aCh, ca := h.subscribe(anna, false)
	defer ca()
	adminCh, cadm := h.subscribe(99, true)
	defer cadm()

	h.publish([]ytdlChange{
		chg("mine", &anna),
		chg("theirs", &bo),
		chg("orphan", nil), // the account was deleted (spec 0013, FR-006d)
	})

	mine := recv(t, aCh)
	if len(mine) != 1 || mine[0].RequestID != "mine" {
		t.Fatalf("a user received %+v, want only their own download", ids(mine))
	}

	all := recv(t, adminCh)
	if len(all) != 3 {
		t.Fatalf("an admin received %v, want all three including the unowned one", ids(all))
	}
}

func ids(cs []ytdlChange) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.RequestID)
	}
	return out
}

// FR-012a and FR-012b. The reconciler admits queued work and captures outcomes
// that exist nowhere else once a worker is swept; a reader that stopped reading
// must not be able to hold that up, and must not be allowed to grow a backlog.
func TestYtdlHub_ADeadReaderIsDroppedRatherThanBlockingThePublisher(t *testing.T) {
	h := newYtdlHub()
	anna := int64(1)
	ch, cancel := h.subscribe(anna, false)
	defer cancel()

	// Fill the buffer and then some, never reading. If publish blocked, this
	// test would hang rather than fail — which is why it is bounded below.
	done := make(chan struct{})
	go func() {
		for i := 0; i < ytdlSubscriberBuffer+5; i++ {
			h.publish([]ytdlChange{chg("a", &anna)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a reader that stopped reading")
	}

	// Drain what was buffered; the channel must then be CLOSED, because a reader
	// that fell behind is dropped and has to find out.
	for range ch {
	}

	if h.hasSubscribers() {
		t.Error("the overrun subscriber is still registered; it would hold a buffer for the life of the process")
	}
}

func TestYtdlHub_NobodyWatchingIsVisible(t *testing.T) {
	h := newYtdlHub()
	if h.hasSubscribers() {
		t.Fatal("a fresh hub reports subscribers")
	}
	_, cancel := h.subscribe(1, false)
	if !h.hasSubscribers() {
		t.Fatal("a live subscription is not counted")
	}
	cancel()
	if h.hasSubscribers() {
		t.Fatal("a cancelled subscription is still counted, so the diff would keep running for nobody")
	}
	cancel() // idempotent: a connection may unsubscribe twice
}

// A nil hub is the no-op case: a deployment that never built one, and every
// test that does not care about streaming.
func TestYtdlHub_NilIsInert(t *testing.T) {
	var h *ytdlHub
	if h.hasSubscribers() {
		t.Fatal("a nil hub claims subscribers")
	}
	h.publish([]ytdlChange{chg("a", nil)}) // must not panic
}

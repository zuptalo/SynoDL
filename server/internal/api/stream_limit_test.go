package api

import (
	"sync"
	"testing"
)

func TestStreamLimiterAdmitsUpToMax(t *testing.T) {
	l := newStreamLimiter(2)
	if !l.acquire() {
		t.Fatal("first acquire should be admitted")
	}
	if !l.acquire() {
		t.Fatal("second acquire should be admitted")
	}
	if l.acquire() {
		t.Fatal("third acquire must be rejected over the cap")
	}
}

func TestStreamLimiterReleasesSlot(t *testing.T) {
	l := newStreamLimiter(1)
	if !l.acquire() {
		t.Fatal("first acquire should be admitted")
	}
	if l.acquire() {
		t.Fatal("second acquire must be rejected at the cap")
	}
	l.release()
	if !l.acquire() {
		t.Fatal("after release a slot must be available again")
	}
}

func TestStreamLimiterFloorsAtOne(t *testing.T) {
	// A non-positive cap must not disable the bound (that would be unbounded
	// streams against the NAS); it clamps to a single slot.
	l := newStreamLimiter(0)
	if !l.acquire() {
		t.Fatal("floored limiter must admit one")
	}
	if l.acquire() {
		t.Fatal("floored limiter must reject the second")
	}
}

// The limiter is touched from every stream goroutine, so it must be race-free.
func TestStreamLimiterConcurrent(t *testing.T) {
	l := newStreamLimiter(50)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.acquire() {
				l.release()
			}
		}()
	}
	wg.Wait()
	// All released — the full cap must be available again.
	for i := 0; i < 50; i++ {
		if !l.acquire() {
			t.Fatalf("slot %d should be free after all releases", i)
		}
	}
	if l.acquire() {
		t.Fatal("cap must still hold after concurrent churn")
	}
}

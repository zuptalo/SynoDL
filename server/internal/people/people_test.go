package people

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"synodl/server/internal/store"
)

// fakeStore is the slice of the database this package uses, and nothing else.
type fakeStore struct {
	mu     sync.Mutex
	photos map[string]store.PersonPhoto
	people map[string]store.SourcePerson
}

func newFakeStore() *fakeStore {
	return &fakeStore{photos: map[string]store.PersonPhoto{}, people: map[string]store.SourcePerson{}}
}

func (f *fakeStore) GetPersonPhoto(id string) (store.PersonPhoto, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.photos[id]
	return p, ok
}

func (f *fakeStore) PutPersonPhoto(id, url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.photos[id] = store.PersonPhoto{URL: url, Found: url != ""}
	return nil
}

func (f *fakeStore) GetSourcePerson(kind, ref string) (store.SourcePerson, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.people[kind+"|"+ref]
	return p, ok
}

func (f *fakeStore) PutSourcePerson(kind, ref string, p store.SourcePerson) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.people[kind+"|"+ref] = p
	return nil
}

// The point of the whole cache: a person seen in one title is not looked up
// again when they turn up in another (FR-024).
func TestPhotoIsLookedUpOnce(t *testing.T) {
	fs := newFakeStore()
	r := New(fs)
	var calls atomic.Int32
	r.lookup = func(context.Context, string) (string, error) {
		calls.Add(1)
		return "https://m.media-amazon.com/images/M/x._V1_.jpg", nil
	}

	for i := 0; i < 5; i++ {
		if got := r.Photo(context.Background(), "nm0000621"); got == "" {
			t.Fatalf("call %d: no photo", i)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("looked up %d times, want 1", calls.Load())
	}
	// And it survives the process: a fresh resolver over the same store asks
	// nobody anything.
	calls.Store(0)
	r2 := New(fs)
	r2.lookup = func(context.Context, string) (string, error) {
		calls.Add(1)
		return "", errors.New("should not be called")
	}
	if got := r2.Photo(context.Background(), "nm0000621"); got == "" {
		t.Fatal("a restart lost the face")
	}
	if calls.Load() != 0 {
		t.Fatalf("a restart caused %d lookups", calls.Load())
	}
}

// Six viewers opening the same title at the same moment is one request to a
// third party, not six (FR-028).
func TestConcurrentDemandCollapsesToOneLookup(t *testing.T) {
	r := New(newFakeStore())
	var calls atomic.Int32
	release := make(chan struct{})
	r.lookup = func(context.Context, string) (string, error) {
		calls.Add(1)
		<-release // hold the first one open so the others must queue behind it
		return "https://m.media-amazon.com/images/M/y._V1_.jpg", nil
	}

	var wg sync.WaitGroup
	got := make([]string, 20)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i] = r.Photo(context.Background(), "nm0000001")
		}(i)
	}
	// Let them all arrive before the one in flight completes.
	for calls.Load() == 0 {
	}
	close(release)
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("looked up %d times, want 1", calls.Load())
	}
	for i, u := range got {
		if u == "" {
			t.Fatalf("waiter %d got nothing", i)
		}
	}
}

// "Nobody has a photograph of this person" is an answer worth remembering, or
// every view of every title they appear in asks again.
func TestMissIsRemembered(t *testing.T) {
	fs := newFakeStore()
	r := New(fs)
	var calls atomic.Int32
	r.lookup = func(context.Context, string) (string, error) {
		calls.Add(1)
		return "", nil // IMDb has no photograph of them
	}

	if got := r.Photo(context.Background(), "nm0000002"); got != "" {
		t.Fatalf("invented a photo: %q", got)
	}
	r2 := New(fs) // a restart
	r2.lookup = r.lookup
	if got := r2.Photo(context.Background(), "nm0000002"); got != "" {
		t.Fatalf("invented a photo: %q", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("a known miss was re-asked: %d lookups", calls.Load())
	}
}

// A failure is NOT the same as a miss: it must not be written down as "this
// person has no face" for a month, or one outage poisons the cache.
func TestFailureIsNotCachedDurably(t *testing.T) {
	fs := newFakeStore()
	r := New(fs)
	r.lookup = func(context.Context, string) (string, error) {
		return "", errors.New("blocked")
	}
	if got := r.Photo(context.Background(), "nm0000003"); got != "" {
		t.Fatalf("got %q", got)
	}
	if _, ok := fs.GetPersonPhoto("nm0000003"); ok {
		t.Fatal("an outage was written to the store as a real answer")
	}
}

// Anything that is not an IMDb person id never reaches an outbound request.
func TestPhotoRejectsNonPersonIDs(t *testing.T) {
	r := New(nil)
	r.lookup = func(context.Context, string) (string, error) {
		t.Fatal("a bad id reached the network")
		return "", nil
	}
	for _, bad := range []string{"", "tt2948372", "nm12", "../nm0000001", "nm0000001/../.."} {
		if got := r.Photo(context.Background(), bad); got != "" {
			t.Fatalf("%q yielded %q", bad, got)
		}
	}
}

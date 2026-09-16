package people

import (
	"container/list"
	"context"
	"sync"
	"time"

	"synodl/server/internal/store"
)

// Store is the slice of the SQLite store this package needs. Small and declared
// at the call site, so a test passes a fake and this package never learns what
// else the database holds.
type Store interface {
	GetPersonPhoto(imdbID string) (store.PersonPhoto, bool)
	PutPersonPhoto(imdbID, photoURL string) error
	GetSourcePerson(kind, ref string) (store.SourcePerson, bool)
	PutSourcePerson(kind, ref string, p store.SourcePerson) error
}

// Resolver answers "what does this person look like", from memory, then from the
// store, and only then from IMDb.
//
// Three layers, each earning its place:
//
//   - The LRU absorbs one sheet asking for six faces and the next sheet asking
//     for three of the same people, without touching SQLite.
//   - The store survives a restart, which is the case that matters: without it,
//     every deploy would re-read a third party's pages for hundreds of people.
//   - Single-flight collapses concurrent demand for the same unresolved person
//     into ONE outbound request, however many viewers asked at once (FR-028).
type Resolver struct {
	store Store

	mu    sync.Mutex
	lru   *list.List
	items map[string]*list.Element

	flightMu sync.Mutex
	flight   map[string]chan struct{}

	// gate bounds outbound lookups instance-wide (FR-021). A buffered channel of
	// tokens rather than a worker pool: the work is one request, and the only
	// thing that needs limiting is how many are in the air at once.
	gate chan struct{}

	// lookup is the outbound call, swappable so tests never reach the network.
	lookup func(ctx context.Context, imdbID string) (string, error)
}

const (
	lruMax = 4096
	lruTTL = 10 * time.Minute
	// maxOutbound is how many photograph lookups may be in flight at once for the
	// whole instance. Small on purpose: this is somebody else's server, and a
	// polite trickle is the difference between being served and being blocked.
	maxOutbound = 4
)

type lruEntry struct {
	key  string
	url  string
	at   time.Time
	miss bool
}

// New builds a resolver over the given store. A nil store is allowed and means
// "memory only" — the stateless mode has no database, and faces are still worth
// showing there.
func New(s Store) *Resolver {
	return &Resolver{
		store:  s,
		lru:    list.New(),
		items:  map[string]*list.Element{},
		flight: map[string]chan struct{}{},
		gate:   make(chan struct{}, maxOutbound),
		lookup: LookupPhoto,
	}
}

// Photo returns a person's photograph URL, or "" when there is none to be had.
//
// It never returns an error, and that is the contract: a face that could not be
// resolved is indistinguishable from a person nobody has a picture of, because
// to the viewer they are the same thing — a tile with initials on it.
func (r *Resolver) Photo(ctx context.Context, imdbID string) string {
	if !rePersonID.MatchString(imdbID) {
		return ""
	}
	if url, ok := r.fromMemory(imdbID); ok {
		return url
	}
	if r.store != nil {
		if p, ok := r.store.GetPersonPhoto(imdbID); ok {
			r.remember(imdbID, p.URL)
			return p.URL
		}
	}
	return r.resolveOnce(ctx, imdbID)
}

// resolveOnce performs the lookup, or waits for the one already running.
func (r *Resolver) resolveOnce(ctx context.Context, imdbID string) string {
	r.flightMu.Lock()
	if done, running := r.flight[imdbID]; running {
		r.flightMu.Unlock()
		select {
		case <-done:
			// Whoever was ahead of us has written the answer down; read it the
			// same way we would have found it in the first place.
			if url, ok := r.fromMemory(imdbID); ok {
				return url
			}
			if r.store != nil {
				if p, ok := r.store.GetPersonPhoto(imdbID); ok {
					return p.URL
				}
			}
			return ""
		case <-ctx.Done():
			return ""
		}
	}
	done := make(chan struct{})
	r.flight[imdbID] = done
	r.flightMu.Unlock()

	defer func() {
		r.flightMu.Lock()
		delete(r.flight, imdbID)
		r.flightMu.Unlock()
		close(done)
	}()

	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return ""
	}

	url, err := r.lookup(ctx, imdbID)
	if err != nil {
		// We could not ask. Remember nothing durably — a block or an outage must
		// not be cached as "this person has no face" for days — but do hold it in
		// memory briefly, so one unreachable IMDb does not mean six stalled
		// lookups per sheet.
		r.remember(imdbID, "")
		return ""
	}
	if r.store != nil {
		_ = r.store.PutPersonPhoto(imdbID, url)
	}
	r.remember(imdbID, url)
	return url
}

func (r *Resolver) fromMemory(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	el, ok := r.items[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*lruEntry)
	if time.Since(e.at) > lruTTL {
		r.lru.Remove(el)
		delete(r.items, key)
		return "", false
	}
	r.lru.MoveToFront(el)
	return e.url, true
}

func (r *Resolver) remember(key, url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if el, ok := r.items[key]; ok {
		e := el.Value.(*lruEntry)
		e.url, e.at = url, time.Now()
		r.lru.MoveToFront(el)
		return
	}
	r.items[key] = r.lru.PushFront(&lruEntry{key: key, url: url, at: time.Now()})
	for r.lru.Len() > lruMax {
		back := r.lru.Back()
		if back == nil {
			break
		}
		r.lru.Remove(back)
		delete(r.items, back.Value.(*lruEntry).key)
	}
}

// SourcePerson returns what is remembered about a source's own handle for
// somebody, and whether anything is.
func (r *Resolver) SourcePerson(kind, ref string) (store.SourcePerson, bool) {
	if r.store == nil {
		return store.SourcePerson{}, false
	}
	return r.store.GetSourcePerson(kind, ref)
}

// RememberSourcePerson records a resolved handle — including one that resolved
// to nothing, which is the answer that most needs remembering: without it, every
// view of every title an unidentifiable person appears in pays for the same
// fruitless page fetch.
func (r *Resolver) RememberSourcePerson(kind, ref string, p store.SourcePerson) {
	if r.store == nil {
		return
	}
	_ = r.store.PutSourcePerson(kind, ref, p)
}

// SetLookupForTest replaces the outbound call. It exists so a handler test can
// exercise this package's behaviour without a network — the production paths
// never call it, and nothing outside a test ever should.
func (r *Resolver) SetLookupForTest(fn func(imdbID string) (string, error)) {
	r.lookup = func(_ context.Context, id string) (string, error) { return fn(id) }
}

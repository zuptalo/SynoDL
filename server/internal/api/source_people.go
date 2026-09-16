package api

import (
	"context"
	"sync"
	"time"

	"synodl/server/internal/source"
	"synodl/server/internal/store"
)

// Resolving the people a source NAMES but cannot identify (spec 0014).
//
// One source publishes an IMDb id beside every person and needs none of this.
// The other links to its own person pages instead, so a title arrives with three
// or four names and no identities — no link to follow, and no way to ask for a
// face. One fetch of a person's own page yields both, and that page is on the
// host the driver just talked to, with the session it already holds.
//
// Three bounds, because this is the one part of the feature that spends the
// operator's source session:
//
//   - only people already NAMED by a title the caller can open reach it, and
//     never a reference a client supplied (FR-014a);
//   - at most maxPersonFetches per title, a few at a time;
//   - the whole batch runs under a deadline, after which whoever is left keeps
//     their name and loses only their link and their face — until the next title
//     they appear in, which finds them in the cache.
const (
	maxPersonFetches   = 8
	personFetchWorkers = 4
	personFetchBudget  = 2500 * time.Millisecond
)

// resolvePeople fills in identities and portraits for every role on a title.
func (d Deps) resolvePeople(ctx context.Context, ref source.SourceRef, td *source.TitleDetail) {
	resolver, ok := ref.Driver.(source.PersonResolver)
	if !ok || d.people == nil {
		return // this source identifies its own people
	}
	roles := []*[]source.Person{&td.Cast, &td.Directors, &td.Creators, &td.Writers}

	// The cache first, for everybody, before any decision about fetching: an
	// evening of browsing resolves the same leads over and over, and the whole
	// point of the cache is that the second title costs nothing.
	var todo []*source.Person
	for _, role := range roles {
		for i := range *role {
			p := &(*role)[i]
			if p.Ref == "" || p.IMDbID != "" {
				continue
			}
			if known, hit := d.people.SourcePerson(ref.Driver.Kind(), p.Ref); hit {
				p.IMDbID, p.PhotoURL = known.IMDbID, known.PhotoURL
				continue
			}
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return
	}
	if len(todo) > maxPersonFetches {
		todo = todo[:maxPersonFetches]
	}

	// The deadline is the promise that this never becomes the reason a sheet is
	// slow. It is separate from the request's own context so a generous client
	// timeout cannot extend it.
	ctx, cancel := context.WithTimeout(ctx, personFetchBudget)
	defer cancel()

	var wg sync.WaitGroup
	sem := make(chan struct{}, personFetchWorkers)
	var mu sync.Mutex
	for _, p := range todo {
		wg.Add(1)
		go func(p *source.Person) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			got, err := resolver.ResolvePerson(ctx, sourceHTTP, ref.Cfg, ref.Sess, p.Ref)
			if err != nil {
				// Not remembered: a page that would not load is not the same as a
				// person with no identity, and caching the former would make one
				// outage stick for a month.
				return
			}
			got, _ = got.Clamp()
			mu.Lock()
			p.IMDbID, p.PhotoURL = got.IMDbID, got.PhotoURL
			name := p.Name
			mu.Unlock()
			// Remembered even when it resolved to nothing — that IS the answer,
			// and re-asking for it on every view of every title they appear in is
			// what this cache exists to prevent.
			d.people.RememberSourcePerson(ref.Driver.Kind(), got.Ref, store.SourcePerson{
				IMDbID: got.IMDbID, PhotoURL: got.PhotoURL, Name: name,
			})
		}(p)
	}
	wg.Wait()
}

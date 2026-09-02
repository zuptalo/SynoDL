package source

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Combining several configured sources into one catalog view (spec 0007).
//
// This is neither a driver's job nor a handler's: a driver knows one site and
// nothing about the others, and a handler should not own concurrency, timeouts
// and failure isolation. So it lives here, on its own, and is unit-testable
// against fake providers with no HTTP at all.

// PerSourceTimeout bounds how long ONE source may hold up a combined query.
// A slow source contributes nothing and is reported as degraded rather than
// making everyone wait (FR-030): with N sources the user's wait is the slowest
// healthy source, not the sum, and never more than this plus overhead.
const PerSourceTimeout = 8 * time.Second

// Cooling-off for a source that keeps failing (FR-031). Re-querying a source
// that is definitely down on every single request slows every search and keeps
// hammering an upstream that is already struggling. After this many consecutive
// failures the source is skipped — still reported as degraded, so the user is
// told rather than silently shown less — until the window elapses and one
// request is allowed through to see whether it recovered.
const (
	coolOffThreshold = 3
	coolOffWindow    = 60 * time.Second
)

// SourceRef binds one configured source to everything needed to call it. The
// session travels with its own source and is never shared between refs.
type SourceRef struct {
	ID     int64
	Name   string
	Driver Provider
	Cfg    Config
	Sess   Session
}

// breaker tracks consecutive failures per source id.
type breakerState struct {
	failures int
	until    time.Time
}

var (
	breakerMu sync.Mutex
	breakers  = map[int64]*breakerState{}
)

// breakerOpen reports whether this source is currently in its cooling-off
// window, and should be skipped without a call.
func breakerOpen(id int64, now time.Time) bool {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	b := breakers[id]
	return b != nil && b.failures >= coolOffThreshold && now.Before(b.until)
}

func breakerFail(id int64, now time.Time) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	b := breakers[id]
	if b == nil {
		b = &breakerState{}
		breakers[id] = b
	}
	b.failures++
	if b.failures >= coolOffThreshold {
		b.until = now.Add(coolOffWindow)
	}
}

func breakerOK(id int64) {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	delete(breakers, id)
}

// ResetBreakers clears all cooling-off state. For tests, and for the moment an
// admin re-pastes a session (the operator has just fixed the thing that was
// failing; making them wait out a cooling-off window would be absurd).
func ResetBreakers() {
	breakerMu.Lock()
	defer breakerMu.Unlock()
	breakers = map[int64]*breakerState{}
}

// SearchAll queries every ref concurrently and interleaves the results.
//
// It does not return an error when a source fails: a failing source must never
// blank a view that other sources could fill (FR-012). Failures come back in
// Degraded. Only when EVERY source fails does the caller have nothing, which it
// can detect from empty items plus a full Degraded list — and report once, not
// once per source.
func SearchAll(ctx context.Context, c *Client, refs []SourceRef, q SearchQuery) SearchResult {
	type outcome struct {
		ref    SourceRef
		res    SearchResult
		reason string // "" = success
	}

	now := time.Now()
	results := make([]outcome, len(refs))
	var wg sync.WaitGroup

	for i, ref := range refs {
		// A source in its cooling-off window is skipped without a call, but still
		// reported — the user learns the source is unavailable rather than quietly
		// seeing a shorter list.
		if breakerOpen(ref.ID, now) {
			results[i] = outcome{ref: ref, reason: ReasonUnreachable}
			continue
		}
		wg.Add(1)
		go func(i int, ref SourceRef) {
			defer wg.Done()
			// One page fetch per source per query — the fan-out is bounded by the
			// number of sources and never multiplies per item (FR-032).
			cctx, cancel := context.WithTimeout(ctx, PerSourceTimeout)
			defer cancel()
			res, err := ref.Driver.Search(cctx, c, ref.Cfg, ref.Sess, q)
			if err != nil {
				reason := classify(err, cctx)
				breakerFail(ref.ID, time.Now())
				results[i] = outcome{ref: ref, reason: reason}
				return
			}
			breakerOK(ref.ID)
			// Stamp provenance before anything merges: once interleaved there is no
			// way to tell which source an item came from, and the client needs it
			// both for the source label and to address the title later.
			//
			// Copy rather than write through res.Items: that slice belongs to the
			// driver, which is free to return cached or shared backing data, and
			// stamping it in place would corrupt whatever else holds a reference —
			// including, with two sources, the other source's copy of the same items.
			stamped := make([]CatalogTitle, len(res.Items))
			copy(stamped, res.Items)
			for j := range stamped {
				stamped[j].SourceID = ref.ID
				stamped[j].SourceName = ref.Name
				stamped[j].ID = QualifyID(ref.ID, stamped[j].ID)
			}
			res.Items = stamped
			results[i] = outcome{ref: ref, res: res}
		}(i, ref)
	}
	wg.Wait()

	var (
		pages    []([]CatalogTitle)
		out      SearchResult
		maxPages int
	)
	for _, o := range results {
		if o.reason != "" {
			out.Degraded = append(out.Degraded, DegradedSource{
				SourceID: o.ref.ID, Name: o.ref.Name, Reason: o.reason,
			})
			continue
		}
		if len(o.res.Items) > 0 {
			pages = append(pages, o.res.Items)
		}
		if o.res.Pages > maxPages {
			maxPages = o.res.Pages
		}
	}
	out.Page = q.Page
	if out.Page < 1 {
		out.Page = 1
	}
	// Pages is the maximum across contributing sources: the list ends when every
	// source is exhausted, not when the shortest one is.
	out.Pages = maxPages
	out.Items = Interleave(pages)
	return out
}

// Interleave merges per-source pages round-robin: the first item of each source,
// then the second of each, and so on, skipping sources that have run out.
//
// This is what guarantees every source appears on the first screenful however
// differently the sources paginate (they return different page sizes). Ordering
// is therefore exact within a source and approximate across them — the deliberate
// trade for not over-fetching, with exact ordering available by selecting a
// single source.
//
// It does NOT de-duplicate. Two sources carrying the same film yield two entries,
// each labelled with its source (FR-005a) — that is the specified behavior, not
// an oversight: the entries offer different releases and different download
// options, and collapsing them would hide one.
func Interleave(pages [][]CatalogTitle) []CatalogTitle {
	total, longest := 0, 0
	for _, p := range pages {
		total += len(p)
		if len(p) > longest {
			longest = len(p)
		}
	}
	if total == 0 {
		return nil
	}
	out := make([]CatalogTitle, 0, total)
	for i := 0; i < longest; i++ {
		for _, p := range pages {
			if i < len(p) {
				out = append(out, p[i])
			}
		}
	}
	return out
}

// classify maps a driver error to a reason category for the client. It never
// carries upstream text: the client gets a category and the source's name.
func classify(err error, ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return ReasonTimeout
	}
	if _, ok := AsNeedsRefresh(err); ok {
		return ReasonNeedsRefresh
	}
	var pv *ErrProviderVerify
	if errors.As(err, &pv) {
		switch pv.Reason {
		case "unsubscribed":
			return ReasonUnsubscribed
		case "invalid_token", "challenge":
			return ReasonNeedsRefresh
		}
	}
	if errors.Is(err, ErrUnsubscribed) {
		return ReasonUnsubscribed
	}
	return ReasonUnreachable
}

// SortRefs orders refs the way the operator arranged them, so a combined list is
// stable between requests. Ties break by id, making the order total even when an
// operator never sets one.
func SortRefs(refs []SourceRef, order func(int64) int64) {
	sort.SliceStable(refs, func(i, j int) bool {
		oi, oj := order(refs[i].ID), order(refs[j].ID)
		if oi != oj {
			return oi < oj
		}
		return refs[i].ID < refs[j].ID
	})
}

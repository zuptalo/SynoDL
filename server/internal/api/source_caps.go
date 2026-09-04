package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/source"
)

// Per-source capability snapshots, and the translation that lets one chosen
// filter value mean the right thing to every source (spec 1024).
//
// Two sources spell the same genre differently — a numeric code on one, a Persian
// word on the other — so the client cannot send one value that both understand.
// It sends whatever the combined facet list offered, and this file rewrites it
// per source before the search fans out.

// capsTTL is how long a source's declared abilities are trusted. They change when
// the site adds a genre, which is rare, and re-reading them costs an upstream
// page fetch — so this is generous. Nothing is persisted: a restart simply
// re-reads.
const capsTTL = 10 * time.Minute

type capsCache struct {
	mu   sync.Mutex
	byID map[int64]capsRec
}

type capsRec struct {
	params source.SearchParameters
	at     time.Time
}

// sourceCaps returns what a source says it can filter and sort by, from cache
// when it is fresh. ok is false when the source could not be asked — the caller
// then knows nothing about it rather than assuming it supports nothing, which is
// a different thing.
func (d Deps) sourceCaps(ctx context.Context, ref source.SourceRef) (source.SearchParameters, bool) {
	if d.caps != nil {
		d.caps.mu.Lock()
		rec, ok := d.caps.byID[ref.ID]
		d.caps.mu.Unlock()
		if ok && time.Since(rec.at) < capsTTL {
			return rec.params, true
		}
	}
	p, err := ref.Driver.Parameters(ctx, sourceHTTP, ref.Cfg, ref.Sess)
	if err != nil {
		// Deliberately not cached: a transient failure must not blank a source's
		// abilities for the whole TTL.
		return source.SearchParameters{}, false
	}
	if d.caps != nil {
		d.caps.mu.Lock()
		if d.caps.byID == nil {
			d.caps.byID = map[int64]capsRec{}
		}
		d.caps.byID[ref.ID] = capsRec{params: p, at: time.Now()}
		d.caps.mu.Unlock()
	}
	return p, true
}

// invalidateCaps drops a source's snapshot, so a re-configured or re-authorised
// source is re-read rather than serving what it could do under an old session.
func (d Deps) invalidateCaps(id int64) {
	if d.caps == nil {
		return
	}
	d.caps.mu.Lock()
	delete(d.caps.byID, id)
	d.caps.mu.Unlock()
}

// translatedFacets are the facets whose values differ between sources and so have
// to be rewritten per source.
//
// Type is deliberately absent: its values are already canonical ("movie",
// "series", "anime") and every driver reads them directly, so translating it
// would replace a working shared vocabulary with a per-source one. The rest of
// the facets are offered by one source only, so they never reach a second source
// that would need them translated — but they are listed anyway, because a third
// source could offer them tomorrow and the alternative is a silent mismatch.
type facetAccessor struct {
	name string
	opts func(source.SearchParameters) []source.FacetOption
	get  func(source.SearchFilters) []string
	set  func(*source.SearchFilters, []string)
}

func translatedFacets() []facetAccessor {
	return []facetAccessor{
		{
			name: "genre",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Genres },
			get:  func(f source.SearchFilters) []string { return f.Genre },
			set:  func(f *source.SearchFilters, v []string) { f.Genre = v },
		},
		{
			name: "score",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Scores },
			get:  func(f source.SearchFilters) []string { return oneOf(f.Score) },
			set:  func(f *source.SearchFilters, v []string) { f.Score = firstOf(v) },
		},
		{
			name: "quality",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Qualities },
			get:  func(f source.SearchFilters) []string { return oneOf(f.Quality) },
			set:  func(f *source.SearchFilters, v []string) { f.Quality = firstOf(v) },
		},
		{
			name: "language",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Languages },
			get:  func(f source.SearchFilters) []string { return oneOf(f.Language) },
			set:  func(f *source.SearchFilters, v []string) { f.Language = firstOf(v) },
		},
		{
			name: "country",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Countries },
			get:  func(f source.SearchFilters) []string { return oneOf(f.Country) },
			set:  func(f *source.SearchFilters, v []string) { f.Country = firstOf(v) },
		},
		{
			name: "channel",
			opts: func(p source.SearchParameters) []source.FacetOption { return p.Channels },
			get:  func(f source.SearchFilters) []string { return oneOf(f.Channel) },
			set:  func(f *source.SearchFilters, v []string) { f.Channel = firstOf(v) },
		},
	}
}

func oneOf(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return []string{v}
}

func firstOf(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// perSourceFilters resolves the filters the client chose into each source's own
// vocabulary.
//
// A ref is absent from the result when nothing needed rewriting for it — the
// shared filters already say what it needs. A ref maps to nil when the user chose
// a value it has no equivalent for; SearchAll then skips it and reports it,
// rather than returning its whole unfiltered catalog into a filtered view.
func (d Deps) perSourceFilters(
	ctx context.Context, refs []source.SourceRef, f source.SearchFilters,
) map[int64]*source.SearchFilters {
	if len(refs) < 2 {
		// One source: whatever the client sent came from that source's own facet
		// list, so there is nothing to translate and no reason to pay for its
		// capability snapshot.
		return nil
	}
	caps := make(map[int64]source.SearchParameters, len(refs))
	for _, ref := range refs {
		if p, ok := d.sourceCaps(ctx, ref); ok {
			caps[ref.ID] = p
		}
	}
	if len(caps) == 0 {
		return nil
	}

	out := map[int64]*source.SearchFilters{}
	// Start every source from the shared filters and rewrite in place.
	for _, ref := range refs {
		cp := f
		out[ref.ID] = &cp
	}

	for _, fa := range translatedFacets() {
		chosen := fa.get(f)
		if len(chosen) == 0 {
			continue
		}
		// What each chosen value MEANS, learned from whichever source recognises it.
		keys := make([]string, 0, len(chosen))
		for _, v := range chosen {
			keys = append(keys, meaningOf(v, refs, caps, fa))
		}
		for _, ref := range refs {
			if out[ref.ID] == nil {
				continue // already ruled out by an earlier facet
			}
			p, known := caps[ref.ID]
			if !known {
				continue // abilities unknown: send what the client sent and let it try
			}
			mapped, ok := mapValues(chosen, keys, fa.opts(p))
			if !ok {
				out[ref.ID] = nil
				continue
			}
			fa.set(out[ref.ID], mapped)
		}
	}
	return out
}

// meaningOf finds the cross-source identity of a raw filter value by looking for
// the option that carries it. "" means no source recognised the value, in which
// case it is passed through untouched — it may be a stored value from a facet
// list that has since changed, and guessing would be worse than trying.
func meaningOf(
	value string, refs []source.SourceRef, caps map[int64]source.SearchParameters, fa facetAccessor,
) string {
	for _, ref := range refs {
		for _, o := range fa.opts(caps[ref.ID]) {
			if o.Value == value {
				return source.FacetKey(o)
			}
		}
	}
	return ""
}

// mapValues rewrites chosen values into the vocabulary of one source's options.
// ok is false when a value that some source understood has no counterpart here.
func mapValues(chosen, keys []string, opts []source.FacetOption) ([]string, bool) {
	out := make([]string, 0, len(chosen))
	for i, v := range chosen {
		key := keys[i]
		if key == "" {
			out = append(out, v) // unrecognised everywhere: pass through
			continue
		}
		var found string
		for _, o := range opts {
			if source.FacetKey(o) == key {
				found = o.Value
				break
			}
		}
		if found == "" {
			return nil, false
		}
		out = append(out, found)
	}
	return out, true
}

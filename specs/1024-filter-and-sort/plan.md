# Implementation Plan: Filter and sort every source the same way

**Spec**: [spec.md](./spec.md) · **Branch**: `feat/1024-filter-and-sort`

## Summary

ZarFilm can filter and sort; SynoDL just never asked it to. Its archive pages
carry a filter panel with a sort order, an IMDb-score band and a genre, and all
three compose with each other and with pagination. The driver declares none of
them, and the one sort it does attempt is written with a parameter the site does
not read.

Teach the ZarFilm driver to declare and honour those three groups, add a sort
capability to the facet set so combined mode can intersect it, and translate the
one facet whose values genuinely differ between sources — genre — at the point
the search fans out.

## Research (done against the live site, 2026-09-04)

Every shape below was verified by fetching it and counting results, not inferred
from markup.

| Capability | Parameter | Values | Verified |
|---|---|---|---|
| Sort | `sortby` | `newest`, `modified`, `popular`, `imdb_rate`, `release` | archives only — **ignored on text search** |
| IMDb score | `imdb_rate` | `9` `8` `7` `6` `5` (above N), `4` (below 5) | archives and text search |
| Genre | `filter_genre` | 29 **Persian** names | `/all-movie/`, `/series/` and text search |

- Genre, score and sort compose, and survive `/page/N/`. Filtering `/all-movie/`
  by comedy took the comedy share of a page from 9/21 to 21/21; adding
  `sortby=imdb_rate&imdb_rate=8` moved the top ratings from 7.6/6.5/6.5 to
  9.8/8.9/8.8.
- **The English slug does not work on the query parameter** — `filter_genre=comedy`
  returns zero cards. English slugs exist only as archive *routes*
  (`/genre/comedy/`), and the series taxonomy is a different route again
  (`/seriegenre/<persian>/`). So the slug is a join key, never a filter value.
- The driver's existing `filter=` is simply the wrong name; `sortby=` is the one
  the site reads. Sorting ZarFilm has never worked.

### What actually needs translating

Comparing the two vocabularies, only one facet genuinely differs:

| Facet | 30nama | ZarFilm | Needs translation? |
|---|---|---|---|
| Type | `movie` / `series` / `anime` | same | no |
| Sort | `favorite` `imdb` `year` `date` | `popular` `imdb_rate` `release` `newest` (+`modified`) | **no** — both drivers already map canonical keys to their own; ZarFilm is just missing two of them |
| Score | `9` `8.5` `8` … | `9` `8` `7` `6` `5` (+`4`) | no — both mean "N and above", so the intersection is the shared subset |
| Genre | numeric codes (`3359` = Comedy) | Persian names (`کمدی`) | **yes** |

So the shared vocabulary is the one the client already speaks, and the only new
machinery is a genre translation step.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)
**Primary Dependencies**: none added — `golang.org/x/net/html` already parses ZarFilm
**Storage**: none. Capability snapshots live in memory with a TTL
**Testing**: `go test ./...` against captured pages and a fake site; vitest; Playwright
**Project Type**: web (Vue PWA + Go proxy)
**Performance Goals**: no upstream call added per search — translation reads a
cached capability snapshot
**Constraints**: no new DSM API, no NAS call, no new persistence, nothing new in logs

## Constitution Check

| Principle | Verdict |
|---|---|
| I — Spec-first | Spec written and researched before this plan. |
| II — TDD | Parser and mapping tests before the parsers; translation tests before the translator. |
| III — Credential boundary | Untouched. No allowlist change, no NAS call, no persistence, no new log line. Capability data is source configuration, not user data. |
| IV — Stateless where it can be | The capability cache is derived, in-memory and rebuildable; losing it costs one fetch. |
| V — Release-note commit subjects | `feat(discover): filter and sort ZarFilm the way you already filter 30nama`. |
| VI — Ionic-first UI | No new component: the filter sheet already renders whatever facets the server reports. |

No violations; no Complexity Tracking entries.

## Design

### 1. Sort becomes a declared capability

`SearchParameters` gains `Sorts []FacetOption`, and `IntersectParameters` folds it
in like every other facet. Each driver declares the canonical sort keys it
honours — 30nama the four it already supports, ZarFilm the four it can express
plus `modified`, which is its own and so appears only when it is selected alone.

The client's hardcoded `SORTS` list in `src/services/source-filters.ts` becomes
the *fallback* for when capabilities have not loaded, exactly as `GENRES` and
`SCORES` already are, with the live list preferred when present.

### 2. Genre translation at fan-out

The client sends one genre value drawn from the shared vocabulary. Before a
search reaches a driver, the API layer rewrites it into that source's own value:

```
shared value ──▶ facet key ──▶ that source's FacetOption.Value
   "comedy"        comedy          30nama: "3359"
                                   ZarFilm: "کمدی"
```

- The lookup comes from the same per-source `Parameters()` snapshot
  `gatherParameters` already fetches, cached in memory with a TTL so browsing adds
  no upstream call.
- Drivers are unchanged: each still receives its own native value, as today.
- A source with no equivalent for the chosen value is **skipped and reported
  degraded**, never sent an unfiltered query whose results would be presented as
  filtered (FR-007, FR-012). This reuses the existing `Degraded` channel; it needs
  one new reason.

`facetKey` currently keys slugs and names in separate namespaces (`slug:comedy`
never equals `name:Comedy`), which would stop the two sources joining if 30nama's
live genres turn out to carry no slug. It is folded into one normalised key space
so a slug and an equivalent label agree.

### 3. ZarFilm declares its capabilities

`Parameters()` fetches `/all-movie/` once — the page carries **both** halves of
the genre mapping:

- the filter panel's 29 `data-filter` values (Persian, what the query parameter
  wants), and
- 60 `/genre/<slug>/` links whose text is the same Persian label (the English
  slug, what the join wants).

Joining them on the Persian label derives the mapping **from the site's own
data** rather than a hand-written table that would rot. A handful of series-only
genres have no movie route (reality-TV, game-show); those get an explicit slug so
they still read in English, and any genre left without one keeps its Persian
label and simply never joins.

### 4. ZarFilm honours them

`Search()` builds `sortby`, `imdb_rate` and `filter_genre` onto the archive path
it already constructs, replacing the `/genre/<slug>/` route (which cannot express
series genres and does not compose with the panel). `filter=` becomes `sortby=`.
Sort is dropped for a text search, because the site ignores it there and offering
it would be the "silently does nothing" trap the driver's own comments warn about.

## Project Structure

```
server/internal/source/
├── source.go            # + SearchParameters.Sorts
├── merge.go             # intersect Sorts; single normalised facet key space
└── providers/
    ├── zarfilm.go       # Parameters(): genres/scores/sorts; Search(): real params
    ├── zarfilm_parse.go # + parseFilterPanel(), parseGenreSlugs()
    └── nama30.go        # + Sorts declaration

server/internal/api/
├── source_multi.go      # capability cache + per-source facet translation
└── source_handlers.go   # translate before fan-out; degrade on no equivalent

src/services/source-filters.ts   # SORTS becomes a fallback, not the truth
src/components/SourceFilterSheet.vue  # render live sorts when present
```

## Phases

**Phase 1 — capability plumbing**: `Sorts` on the facet set, intersect, key space.
**Phase 2 — ZarFilm**: panel + slug parsers, `Parameters()`, `Search()`.
**Phase 3 — translation**: capability cache, genre rewrite, degrade path.
**Phase 4 — client**: live sorts, fallbacks.
**Phase 5 — gates**: full suite, roadmap, PR.

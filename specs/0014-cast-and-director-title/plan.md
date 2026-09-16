# Implementation Plan: Who made it — cast and director on a title

**Branch**: `feat/0014-cast-and-director-title` | **Date**: 2026-09-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0014-cast-and-director-title/spec.md`

## Summary

Both configured sources publish who made a title and SynoDL discards it. This
adds a people section below the download options on the Discover detail sheet:
cast with characters, then director, creator and writers, each person a tile that
links to their IMDb page.

The work is in four layers:

1. **Drivers** learn to report people. 30nama gains one new API call
   (`single/id/{id}`, fetched in parallel with the download call it already makes)
   which also fills in the synopsis, year and IMDb id that source has always
   returned empty. ZarFilm reads the block already present in the page it fetches,
   and — because that page names people without identifying them — resolves each
   unknown person's page once to learn their IMDb id and portrait.
2. **A `people` package** remembers what was resolved: an LRU and a single-flight
   guard over two new SQLite tables, keyed by the person, so the same actor in the
   next title costs nothing and survives a restart.
3. **A third image proxy** (`GET /v1/source/person/{imdbId}/photo`) fills the gaps
   the sources leave — every crew face, and roughly half of a typical cast — by
   reading `og:image` off IMDb. It is modelled on `handleYtdlThumb`, with its own
   host allowlist, and takes a *person id* rather than a URL so it cannot name a
   host at all.
4. **The sheet** renders tiles from stock Ionic pieces, degrading to initials
   whenever no photograph exists or one fails to load.

The ordering principle throughout: the names are free and must never be blocked
by the faces. A total IMDb outage costs photographs and nothing else.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: none added. `golang.org/x/net/html` (already present)
parses ZarFilm's person pages; the IMDb `og:image` read is a bounded stdlib scan.
Single-flight is ~20 lines in-package rather than `golang.org/x/sync` — adding a
module is a spec-level decision in this repo and this does not warrant one.

**Storage**: the single SQLite database (`DATA_DIR`), migration 39 — two cache
tables, unencrypted because they hold only derived public facts.

**Testing**: `go test ./...` against `httptest` fakes (drivers, people package,
handler) with captured-page fixtures; vitest for the two new pure client modules;
Playwright against the stateful stack, driving the in-repo mock sites and a new
mock IMDb.

**Target Platform**: the single container; Discover detail sheet on mobile-first
PWA.

**Project Type**: web (Go service + Vue PWA in one repo, one image).

**Performance Goals**: the detail sheet renders in the time it does today
(SC-002). 30nama pays no extra round trip (parallel call); ZarFilm pays at most
one bounded, deadline-capped batch of person fetches on a cold cache and none on a
warm one. Photographs arrive independently of the sheet.

**Constraints**: IMDb lookups ≤4 concurrent instance-wide, page read ≤256 KB and
stopped at `</head>`, image ≤8 MB, endpoint rate-limited per IP. ZarFilm person
resolution ≤8 per title, ≤4 concurrent, ≤2.5s for the batch.

**Scale/Scope**: a few thousand distinct people per instance over time; two cache
tables capped at 20 000 rows each.

## Constitution Check

*GATE: passed before Phase 0, re-checked after Phase 1 design.*

| Principle | Assessment |
|---|---|
| **I. Spec-Driven** | Spec 0014, clarified 2026-09-16, this plan, tasks + analyze to follow. Branch, commits, issues and PR all carry the id. ✅ |
| **II. TDD** | `tasks.md` orders failing tests first: driver parse tests against captured fixtures, people-cache TTL/single-flight tests, endpoint validation tests, then implementations. New user-facing behaviour ⇒ new e2e specs. The two new pure client modules (`imdbPersonUrl`, `initials`) join the vitest coverage allowlist — a ratchet up, never down. ✅ |
| **III. Custodial State & Credential Safety** | **Engaged on both counts**, and answered in the spec's Credential-Safety Impact. One store, one volume: two tables in the existing SQLite database, no second datastore. No secret stored ⇒ no `SECRETS_KEY` involvement, and nothing may be added to these tables that would need it. Nothing touches the NAS, the DSM allowlist, or a worker. The new outbound hosts are a **fixed, compiled-in, feature-local** allowlist — deliberately not merged into the poster proxy's — and the only client input is an id shape that cannot express a host. Names, characters and URLs are content: never logged. ⚠️ ⇒ `/speckit-checklist` is REQUIRED (see Gate sequencing). |
| **IV. Offline-First Client Data** | No IndexedDB change: people are display data for an open sheet, exactly like the download options beside them. No `DB_VERSION` bump. ✅ |
| **V. Quality Gates** | `npm run build`, `npm run test:unit:coverage`, `go build/vet/test`, `npm run test:e2e` all green before done. User-facing commit type ⇒ plain-language release-note subject. ✅ |
| **VI. Ionic-First UI** | Tiles are `ion-avatar` + `ion-label`; section headings are `ion-list-header`. The only custom CSS is the horizontal scroll container, which is layout, not a re-implemented widget — no Ionic primitive provides a horizontally scrolling row. Existing `--app-*` tokens only; verified light, dark and RTL. ✅ |
| **VII. Traceable Delivery** | `ROADMAP.md` regenerated from the `**Status**:` line; one issue per task via `taskstoissues`; `Closes #N` for each in the PR. ✅ |

**Domain constraints**: single image and one state volume — unchanged, no sidecar,
no new volume, no worker. Mock-DSM dev parity is extended rather than broken: the
whole feature, IMDb included, is exercisable with no network (research R6).

**No violations to justify** — the Complexity Tracking table is therefore omitted.
The two judgement calls that *could* look like violations are both argued in
research and land on the conservative side: a third image proxy rather than a
widened second one (R4), and a persisted cache rather than a derived one (R5,
which Principle III's own "the request and its finished outcome may be durable"
carve-out anticipates).

## Project Structure

### Documentation (this feature)

```text
specs/0014-cast-and-director-title/
├── spec.md
├── plan.md              # this file
├── research.md          # R1–R7: what the sources actually publish
├── data-model.md        # types, schema, lifetimes
├── quickstart.md        # how to see it working
├── contracts/
│   └── person-photo.md
├── checklists/
│   ├── requirements.md
│   └── security.md      # REQUIRED (Principle III) — /speckit-checklist
└── tasks.md             # /speckit-tasks
```

### Source code

```text
server/
├── internal/source/
│   ├── source.go                      # + Person, TitleDetail.{Cast,Directors,Creators,Writers,Year},
│   │                                  #   PersonResolver
│   └── providers/
│       ├── nama30.go                  # + single/id/{id} call, credits, plot/year/imdb
│       ├── zarfilm.go                 # + person-page resolution (bounded, deadlined)
│       ├── zarfilm_parse.go           # + parseCredits, parsePersonPage
│       ├── imdbbase_dev.go            # NEW, //go:build sourcemock
│       ├── imdbbase_prod.go           # NEW, //go:build !sourcemock
│       └── testdata/
│           ├── tn_single.json         # NEW — captured shape, movie + series
│           └── zarfilm/
│               ├── credits.html       # NEW
│               └── person_*.html      # NEW (real portrait / theme stand-in / no imdb link)
├── internal/people/                   # NEW package
│   ├── people.go                      # LRU + single-flight + store-backed resolve
│   ├── imdb.go                        # og:image scan, host rule, size rewrite
│   └── *_test.go
├── internal/store/
│   ├── schema.go                      # migration 39
│   ├── person_repos.go                # NEW
│   └── migrations_golden_test.go      # + checksum
├── internal/api/
│   ├── source_person.go               # NEW — GET /v1/source/person/{id}/photo
│   ├── source_handlers.go             # resolve refs before responding
│   └── router.go                      # + route (rate-limited)
└── internal/synomock/
    ├── sources.go                     # + tn single endpoint, zar credits + person pages
    └── imdbmock.go                    # NEW — /mockimdb/name/{nm}/, /mockimg/{nm}.jpg

src/
├── components/
│   ├── SourceCredits.vue              # NEW — the people section
│   └── SourceTitleModal.vue           # renders it below the download options
└── services/
    ├── imdb-link.ts                   # + imdbPersonUrl
    ├── person.ts                       # NEW — initials()
    └── api.ts                         # + personPhotoSrc()

e2e/stateful/
└── discover-credits.spec.ts           # NEW
```

**Structure Decision**: no new top-level anything. The one new server package
(`internal/people`) exists because remembering a person is neither a driver's job
(drivers are stateless by contract and receive no store) nor a handler's (the
resolution is shared by the detail path and the photo endpoint).

## Phase notes

**Phase 0 — research** (`research.md`): complete. R1/R2 pin what each source
publishes and what it costs; R3 pins the IMDb read; R4 settles the third proxy;
R5 settles the cache; R6 settles no-network testability; R7 settles the
incidental synopsis/year/imdb gain.

**Phase 1 — design** (`data-model.md`, `contracts/`, `quickstart.md`): complete.

**Phase 2 — tasks**: `/speckit-tasks`, ordered tests-first and grouped by user
story so US1 (names only) is shippable on its own, then US2/US3 (faces), US4
(links), US5 (initials), US6 (cache), US7 (incidental gain).

Sequencing constraint worth stating once: **US1 must be independently green**. If
every photograph path is removed, the sheet still names everyone. The e2e suite
asserts that directly by running one spec with the mock IMDb switched off.

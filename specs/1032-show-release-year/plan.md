# Implementation Plan: Show release year and readable genres on titles

**Branch**: `feat/1032-show-release-year` | **Date**: 2026-09-07 | **Spec**: [spec.md](./spec.md)

## Summary

Spend data the app already fetches. The year is parsed by one driver and dropped
on the floor; the client separately derives a year from the title string and
discards that too. Genres arrive from one source as English slugs and from the
other as Farsi display names — while that same driver already builds the
Farsi→English mapping for its filter menu and uses it nowhere else.

So this is mostly wiring, plus one honest guard: a source known to publish
implausible years must not be allowed to put them on a card.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: None added. Both halves reuse existing machinery —
`parseGenreSlugs` (ZarFilm), `facet-labels.ts` slug title-casing, `title-year.ts`.

**Storage**: None. No migration, no new column, no cache.

**Testing**: Go table tests per driver against the existing HTML/JSON fixtures;
vitest for the two new pure client modules; Playwright for the rendered caption.

**Performance Goals**: Zero additional source requests and no measurable change
to first-page latency (FR-012) — every input is already in the response being
parsed.

**Constraints**: The card caption has a fixed line budget (FR-008); the title
must never truncate to make room for metadata.

## Constitution Check

| Rule | How this satisfies it |
|---|---|
| Custodial state, one volume | Nothing is stored. No migration. |
| DSM allowlist | Untouched — no NAS call, no DSM API. |
| No new outbound surface | No new host, no extra request; genres and years come from responses already being parsed. |
| Ionic-first UI | Caption changes reuse the existing card markup and theme tokens; no new widget. |
| TDD | Each driver change is preceded by a fixture-based test; the two pure client modules are unit-tested before use. |
| Credential-Safety Impact | Present in the spec; nothing stored, nothing logged, no secret touched. |

**Gate result**: PASS, nothing in Complexity Tracking.

## Project Structure

```text
server/internal/source/
├── source.go                      # CatalogTitle gains Year
└── providers/
    ├── zarfilm.go                 # populate Year; map card genres to slugs
    ├── zarfilm_test.go            # + assertions
    ├── nama30.go                  # populate Year where available
    └── nama30_test.go             # + assertions

src/
├── services/
│   ├── api.ts                     # CatalogTitle gains year
│   ├── genre-label.ts             # NEW pure: slug/name → English label
│   ├── genre-label.test.ts
│   ├── release-year.ts            # NEW pure: plausibility + title fallback
│   └── release-year.test.ts
├── views/tabs/BrowserPage.vue     # caption: year + genre, type suppressed
└── components/SourceTitleModal.vue# header: year alongside type + rating
```

**Structure Decision**: The two judgement calls — what counts as a plausible
year, and how a genre becomes an English label — go in their own pure modules
rather than inline in a template. They are exactly the kind of logic that earns
unit tests, and the vitest coverage floors already gate `src/services/`.

## Phase 0 — Research

Settled by reading the code before the spec was written; no open questions.

- **ZarFilm parses a per-card year** (`byClass("year")`) and drops it when
  building `CatalogTitle`. Free once the field exists.
- **ZarFilm already maps Farsi genre labels to English slugs** via
  `parseGenreSlugs`, applied today only to facets. Its own comment says the slug
  "is what lets this genre join with another source's, and what the client
  title-cases for display" — this spec is that sentence being carried out.
- **30nama already emits English slugs** as card genres (`genreNames()` returns
  `g.Slug`), so it needs no genre change.
- **30nama's years are unreliable** — spec 2006 documented a body of titles with
  broken years, which is why FR-006 exists rather than trusting the source.
- **The client already computes a title-derived year** (`splitYear`) and
  discards it, giving a fallback for titles whose source publishes no year field.

## Phase 1 — Design

- `CatalogTitle.Year` is a **string**, not an int: a series carries a range
  ("2008 – 2013") and an ongoing one an open range. A number cannot hold that,
  and the client already formats ranges this way.
- Genre display is **slug-first**: use the slug when present, fall back to the
  source's own name, then to the raw value — the same precedence
  `facet-labels.ts` already applies, extracted so cards and filters cannot drift.
- The year shown is **the source's field first, the title-derived year second**,
  and neither if the result is implausible.

# Implementation Plan: ZarFilm titles carry an IMDb link and a synopsis

**Spec**: [spec.md](./spec.md) · **Branch**: `feat/1023-zarfilm-titles-carry`

## Summary

The title sheet renders its header metadata — synopsis, genres, rating, IMDb link
— from the **catalog entry** it was opened with, not from the title response.
30nama's search results carry `plot` and `imdbId`, so 30nama titles get the full
header for free. ZarFilm's listing pages publish neither, so its sheets have been
bare since the source was added.

Rather than teach the ZarFilm listing parser to invent metadata it does not have,
carry the two values on the **title detail response**, which ZarFilm can populate
for free: `zarfilm.Title()` already fetches and parses the title page, and that
page carries both the synopsis and a real IMDb link. The sheet then merges detail
values in behind whatever the catalog entry already supplied.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)
**Primary Dependencies**: `golang.org/x/net/html` (already a dependency — the
ZarFilm driver exists because the site publishes no API)
**Storage**: none — nothing about this feature is persisted
**Testing**: `go test ./...` against checked-in page fixtures; vitest; Playwright
**Target Platform**: same container as today
**Project Type**: web (Vue PWA + Go proxy)
**Performance Goals**: no additional request to the source; parsing rides on the
page fetch the title request already performs
**Constraints**: no new DSM API, no new NAS call, no new persistence, nothing new
in logs
**Scale/Scope**: two parser functions, two wire fields, one merge in the sheet

### Where the values come from

Both are read from the title page `zarfilm.Title()` already has in hand:

| Value | Source markup | Note |
|---|---|---|
| IMDb id | `imdb.com/title/tt…` anchor in the page's IMDb block | `parseIMDbID` already exists and is already tested — it is simply not called from `Title()` |
| Synopsis | `div.plot` | The site repeats the same text in a `div.mobile_plot` for its narrow layout, so take the first non-empty of the two and never concatenate |

`parseIMDbID` falls back to a bare `tt…` match anywhere in the body. On a **detail**
page that fallback is safe and useful — the page's own poster filename embeds the
title's id. It is not safe on a *listing* page, where 98 different ids appear, and
this plan does not call it there.

## Constitution Check

| Principle | Verdict |
|---|---|
| I — Spec-first | This document; spec written before code. |
| II — TDD | Parser tests land before the parser; the wire-shape test before the field is read. |
| III — Credential boundary | Untouched: no allowlist change, no NAS call, no new persistence, no new log line. See the spec's Credential-Safety Impact. A `/speckit-checklist` run is **not** required — the boundary is not approached, let alone widened. |
| IV — Stateless where it can be | The two values are computed per request from a page fetch already being made and are never cached or stored. |
| V — Release-note commit subjects | Subject: `feat(discover): read what a ZarFilm title is about before downloading it`. |
| VI — Ionic-first UI | No new component; the existing `.plot` paragraph and `.imdb-link` anchor are reused as-is, with one attribute added. |

No violations; no Complexity Tracking entries.

## Project Structure

### Documentation (this feature)

```
specs/1023-zarfilm-titles-carry/
├── spec.md
├── plan.md      # this file
└── tasks.md
```

No `data-model.md`, `contracts/` or `research.md`: the feature adds two optional
fields to one existing response and introduces no entity, no endpoint and no
unresolved question.

### Source Code

```
server/internal/source/
├── source.go                       # + TitleDetail.IMDbID, TitleDetail.Plot
└── providers/
    ├── zarfilm_parse.go            # + parsePlot()
    ├── zarfilm_parse_test.go       # + parsePlot / parseIMDbID cases
    ├── zarfilm.go                  # Title(): populate both, for movie and series
    ├── zarfilm_test.go             # + detail carries metadata
    └── testdata/zarfilm/
        ├── movie_meta.html         # NEW — real capture of a movie page's metadata block
        └── series_meta.html        # NEW — real capture of a series page's metadata block

src/
├── services/api.ts                 # + TitleDetail.imdbId, TitleDetail.plot
└── components/SourceTitleModal.vue # merge detail metadata behind the catalog entry; dir="auto"
```

### Fixtures

The checked-in ZarFilm page fixtures predate the site's current metadata block:
they carry the IMDb anchor but no `div.plot` at all. They are therefore kept
exactly as they are and used as the **negative** case — a page with no synopsis
must yield no synopsis — and two new small fixtures are captured from the live
site (one movie, one series) holding the real metadata block, so the parser is
tested against markup the site actually serves rather than markup invented to
match the parser.

## Design decisions

1. **Driver-populated, not handler-decorated.** `TitleDetail.Ownership` and
   `.Seasons` are set by the API layer on the way out and carry a comment saying
   drivers never set them. These two are the opposite: they are the source's own
   description of the title, so the driver sets them and the handler does not
   touch them. The distinction is worth a comment in `source.go` so the next
   reader does not assume the decoration pattern applies to the whole struct.

2. **Catalog entry wins (FR-008).** The sheet merges as
   `catalog value || detail value`. 30nama's catalog entry already carries an
   English synopsis; a future 30nama `Title()` that returned something thinner
   must not be able to replace it. The merge happens in the sheet's `info`
   computed, which is already the single place the sheet decides what metadata to
   render.

3. **`dir="auto"` rather than `dir="rtl"`.** The synopsis is Persian today but the
   field is generic — a source could return English. `auto` lets the browser pick
   per string from its first strong character, so an English synopsis is not
   forced into right-to-left and a Persian one is not left in left-to-right. The
   attribute goes on the paragraph only, so the rest of the sheet keeps its
   direction.

4. **Text, not markup (FR-007).** The synopsis is extracted with the existing
   `text()` walker, which concatenates text nodes and so drops any tags, and the
   HTML tokenizer resolves entities on the way in. Vue interpolation escapes on
   the way out. Nothing is ever rendered as HTML.

## Phases

**Phase 0 — research**: none needed. The page shape was confirmed against the live
site for both a movie and a series before this plan was written; the selectors are
in the table above.

**Phase 1 — server**: fixtures → parser tests → `parsePlot` → wire fields →
`Title()` populates them → driver test.

**Phase 2 — client**: type → merge → `dir="auto"` → build.

**Phase 3 — gates**: full suite, roadmap, PR.

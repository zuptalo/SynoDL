# Phase 0 — Research

**Feature**: 0008 — Show which Discover titles you already have

Every finding below is grounded in code already in this repository; file references are the
evidence, not illustrations.

---

## R1 — What an on-disk title folder is actually named

**Decision**: Treat a folder name as a *derivative of the catalog title string*, not as a scene
release name, and build matching around exact comparison with a normalisation fallback.

**Rationale**: `handleSourceSend` builds the destination as
`relParent + "/" + sanitizeFolderName(body.Title)`
(`server/internal/api/source_handlers.go:556-568`), where `body.Title` is the raw catalog title
the client displayed. `sanitizeFolderName` (`:792`) only replaces filesystem-hostile characters
(`/ \ : * ? " < > |`, control chars), collapses whitespace, trims `" ."`, and caps at 120 chars.

So for anything SynoDL downloaded, `folderName == sanitizeFolderName(catalogTitle)` — a
comparison, not an inference. `src/services/task-title.ts` already relies on exactly this
property to label task rows from their destination folder.

**Alternatives considered**:
- A general scene-release parser (`parse-torrent-name` / `guessit` style). Rejected: no such
  parser exists in the repo (confirmed — nothing parses `PROPER`/`REPACK`, codec, or group), it
  would be a new dependency or a large hand-rolled one, and it solves a problem this codebase
  does not have, because the folder names are ours.
- Matching on IMDb id. Rejected: `CatalogTitle.IMDbID` exists, but nothing writes it to disk, so
  it cannot identify a pre-existing folder — the exact case the feature is for.

---

## R2 — Normalisation and the rule that prevents false positives

**Decision**: Reduce both sides to a comparison key — split the trailing year/range off, strip a
leading article and bracketed extras, lowercase, then keep only Unicode letters and digits. Match
on the key, and **when both sides carry a year, require the start years to agree** (FR-005).

**Rationale**: The catalog embeds the year at the end of the title, and
`src/services/title-year.ts` already encodes exactly which forms occur:

```
/\s*\(?\b((?:19|20)\d{2})\b(?:\s*[-–]\s*((?:19|20)\d{2})?)?\)?\s*$/
```

covering `Esther 1986`, `(2014)`, `2008 - 2013`, `2008–2013`, and the open-ended `2019 -`. That
regex is the specification for the year half of the key; the Go port must stay behaviourally
identical, and `title-year.test.ts` is the corpus to port with it.

The year-agreement rule exists because key-only matching collapses `It 1990` into `It 2017`.
A false positive is the worst outcome this feature can produce — it makes a user skip something
they wanted — so the rule is asymmetric on purpose: a year mismatch blocks the match, but a
*missing* year on either side does not (a folder named plainly `Friends` should still match).

**Non-Latin scripts are a correctness requirement, not a nicety**: the configured sources serve
Persian titles (`server/internal/source/providers/zarfilm_parse.go` replaces Persian digits).
Normalisation must therefore filter on `unicode.IsLetter`/`unicode.IsDigit`, never on an ASCII
range — an ASCII filter would reduce every Persian title to the empty string and make them all
collide.

**Alternatives considered**: fuzzy distance matching (Levenshtein / trigram). Rejected: it trades
the one failure mode we cannot accept (false positives) for the one we can (a missed match), and
it needs a threshold nobody can justify. Deterministic normalisation is testable; a threshold is
a guess.

---

## R3 — How seasons sit on disk

**Decision**: Support both layouts from a single listing of the title folder — season
subdirectories, and episode files directly inside it.

**Rationale**: SynoDL's own sends produce the **flat** layout: every season of a series lands in
one folder named after the title (`folderName` does not vary by season, `source_handlers.go:565`),
so seasons exist only inside file names. Content that arrived by other means overwhelmingly uses
the **nested** `Season N` layout. Both must work, or the feature covers only half its own subject.

Season/episode extraction has settled prior art here — `SE_RE` in `src/services/task-title.ts`,
ported to Go in `server/internal/tasktitle/tasktitle.go`, which is documented as needing to stay
behaviourally in sync and carries a shared corpus. Its bounding on non-alphanumerics rather than
`\b` is deliberate and load-bearing: sources separate with `_`, and `\b` treats `_` as a word
character, so `X_Men_97_S01E01_1080p` would never match. The new parser reuses that form rather
than inventing a second one. `seasonNum()` in `src/services/quality-sort.ts` already turns a
`Season 6` label into `6` on the client side.

**Episode counts are best-effort.** For the nested layout, counting means one listing per season
directory. Presence is what the spec guarantees (FR-016); counts are gathered with a bounded
concurrent descent (stdlib `sync.WaitGroup` + a semaphore — no new dependency) and a count of
zero simply means "not counted", never "empty".

---

## R4 — Which NAS call, and why no new API is authorized

**Decision**: Use `SYNO.FileStation.List`. Keep `ListFolder` (`filetype=dir`) for the parent
scan, and add **one** new client method, `ListEntries`, that issues the same API and method with
**no `filetype` parameter**, returning directories and files together.

**Rationale**: `SYNO.FileStation.List` is already on the allowlist (`server/internal/syno/http.go:25`)
and already in production use for the destination picker, so **no new DSM API is added**. DSM's
`filetype` defaults to *all*, so omitting it returns both kinds in one round-trip — which is
exactly what season detection needs (season subdirectories *and* episode files), at half the
calls of a dir-listing plus a file-listing.

This is nonetheless a real widening of what SynoDL reads — from directory names to file names —
which is why the spec carries a Credential-Safety Impact section and `/speckit-checklist` is
mandatory before implementation.

`ListShares` is not involved: parents are share-relative paths (`movie`, `tv-show`), so the scan
lists `"/" + parent` directly, as `Deps.validDestinations` already does
(`server/internal/api/destination_handlers.go:99-144`).

**Mock gap found**: `synomock` models `folders map[string][]string` and emits `isdir:true` for
every entry, ignoring `filetype` entirely (`server/internal/synomock/synomock.go:65, 380-437`).
It has no files and no folder-seeding control endpoint — fixtures are hardcoded in
`resetLocked()`. Both must be built, or Principle II's e2e obligation and the mock-DSM dev-parity
Domain Constraint cannot be met.

---

## R5 — Freshness of the snapshot

**Decision**: Hold the snapshot in process memory with a **5-minute TTL**, and discard it
immediately after a successful send. (Clarified with the user, Session 2026-09-03.)

**Rationale**: Re-reading per catalog page would put a NAS round-trip in front of every
infinite-scroll fetch, violating SC-009. Reading once per process makes out-of-band additions
invisible for the life of the container. The TTL bounds staleness at 5 minutes for out-of-band
changes (FR-010a) while explicit invalidation on send makes SynoDL's *own* downloads appear
immediately (FR-008) — which is the case a user will actually notice and test.

**Why memory and not a table**: Principle III keeps download state out of the store — the NAS is
the source of truth. A cache in memory needs no migration, cannot drift across a restart, and
cannot become a durable copy of the user's library. This is also why the snapshot is rebuilt
rather than incrementally maintained.

**Alternatives considered**: a background refresh ticker. Rejected for now — it does NAS work for
an instance nobody is browsing, and lazy refresh-on-use gets the same bound with less machinery.

---

## R6 — Where the flag joins the response

**Decision**: Add `InLibrary` to `source.CatalogTitle` and set it in `handleSourceSearch`
immediately before the response is written.

**Rationale**: `CatalogTitle` already carries app-level fields that no driver populates —
`SourceID` and `SourceName` are stamped by the merge layer (`server/internal/source/merge.go`),
not by `thirtynama.go` or `zarfilm.go`. `InLibrary` is the same kind of field and follows the
same path, so drivers stay unaware of it.

The decorate-just-before-writing shape mirrors `decorateTasks` in
`server/internal/api/task_ownership.go:56`, which joins `SourceDownloads()` onto the task list by
destination. Reusing a recognised pattern beats inventing a second one.

---

## R7 — Where the hide-owned preference lives

**Decision**: A new `hide_owned` column on the existing `source_prefs` table, carried through the
existing `GET/PUT /v1/source/prefs` shape.

**Rationale**: FR-024 requires the setting to follow the user across devices, which rules out
IndexedDB. `source_prefs` is already the per-user Discover view state — `preferred_quality`,
`filters`, `sort`, `sort_order`, `selected_source` — and each of those arrived as an additive
`ALTER TABLE` (migrations 0017, 0018 in `server/internal/store/schema.go`). This is the same
change shape, on the same table, through accessors that already exist.

As a consequence, Principle IV's `DB_VERSION` bump does **not** apply: no IndexedDB object store
is added or altered (`src/db/idb.ts` stays at version 1).

**Alternative considered**: an entry in the existing `filters` JSON blob. Rejected — that column
holds provider facet filters that are sent upstream to the source; a local display preference has
different semantics and would have to be stripped before every search.

---

## R8 — Rejected data sources for "already downloaded"

**`source_downloads.catalog_id`** (`server/internal/store/source_repos.go:367`) already records
every Discover send, keyed by destination folder, carrying `catalog_id`, title, year, poster and
owner. It is exact and free of NAS calls.

**Rejected as the primary signal**, because it is blind to precisely the content the user asked
about: anything on the NAS that predates SynoDL or arrived another way (FR-002, SC-002). It also
holds `catalog_id = ''` for rows written before spec 1016, and its ids are source-qualified
(`<sourceId>:<titleId>`, `server/internal/source/ids.go`), so the same title from a second source
would not match.

It remains a sound **secondary** signal — a title whose folder was later renamed would still be
recognised — and is recorded here as the obvious first extension if renames prove to be a real
problem. It is deliberately out of scope for this spec.

**`download_history`** was rejected outright: it is an append-only statistics log keyed by
destination and task name with no catalog id, built for the Statistics section, not for identity.

---

## R9 — Client-side filtering and backfill

**Decision**: Filter owned titles out inside `useSourceCatalog.fetchPage()`, and when the filter
leaves the grid underfull, fetch further pages until it is filled or the catalog is exhausted
(FR-023a).

**Rationale**: `fetchPage()` already filters `comingSoon` items out of the returned page
(`src/composables/useSourceCatalog.ts`, ~line 272), so the pattern, its interaction with
`pages`/`hasMore`, and its tests already exist; hide-owned joins it rather than adding a second
filtering stage. Backfill is needed because the source cannot filter on ownership upstream — it
knows nothing about the NAS — so a page of 20 results can arrive almost entirely owned.

`loadMore()` already pulls two pages per trigger (`PAGES_PER_LOAD = 2`), so the machinery for
"fetch more before showing" is in place. The backfill must be bounded so an exhausted or
heavily-owned catalog cannot spin.

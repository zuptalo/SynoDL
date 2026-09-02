# Tasks: Multiple Download Sources

**Feature**: 0007-multiple-download-sources | **Branch**: `feat/0007-multiple-download-sources`
**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/source-api.md](./contracts/source-api.md)

Test tasks are included and non-optional: the constitution mandates TDD (Principle II), and the
main risk in this feature is a refactor breaking the source that already works for real users.

**Format**: `- [ ] [ID] [P?] [Story?] Description with file path` — `[P]` marks tasks touching
different files with no incomplete dependency, safe to run in parallel.

---

## Phase 1: Setup

- [ ] T001 Add `golang.org/x/net/html` to `server/go.mod`, then confirm `cd server && go mod tidy && go build ./...` is clean
- [ ] T002 [P] Verify the container build still succeeds with the new dependency in `Dockerfile`
- [ ] T003 [P] Save trimmed zarfilm captures (movie page, series page, archive page, search page, logged-out page, paywalled page) as fixtures in `server/internal/source/providers/testdata/zarfilm/`

---

## Phase 2: Foundational — characterize, then lift the singleton

**Blocking**: every user story depends on this phase. Nothing here changes user-visible
behavior; T004–T006 exist purely so the refactor cannot silently break the existing source.

- [ ] T004 [P] Add characterization tests pinning current single-source search/title/send behavior in `server/internal/api/source_handlers_test.go`
- [ ] T005 [P] Add characterization tests pinning current store accessor behavior in `server/internal/store/source_repos_test.go`
- [ ] T006 [P] Add a test asserting an existing single-provider database upgrades with its sealed session still readable, in `server/internal/store/source_repos_test.go`
- [ ] T007 Append the three additive migrations (`source_providers.sort_order`, `source_providers.last_error`, `source_prefs.selected_source`) to `server/internal/store/schema.go`
- [ ] T008 Replace singleton accessors with `ListProviders` / `GetProviderByID` / `CreateProvider` / `UpdateProvider` / `DeleteProvider` in `server/internal/store/source_repos.go`
- [ ] T009 Add the `unsubscribed` provider state and `last_error` persistence in `server/internal/store/source_repos.go`
- [ ] T010 Reshape `source.Session` to a declared field bag plus `UserAgent`, and add `SessionField` / `SessionFields()` to the `Provider` interface in `server/internal/source/source.go`
- [ ] T011 Migrate sealed session blobs from the old fixed field names into the bag on read, losslessly, in `server/internal/store/source_repos.go`
- [ ] T012 Remove hardcoded provider auth headers from `Client.Do` and let each driver supply its own headers and cookies per request in `server/internal/source/client.go`
- [ ] T013 Move 30nama's auth header construction into the driver with no behavior change in `server/internal/source/providers/thirtynama.go`
- [ ] T014 Confirm T004–T006 still pass unchanged against the refactored store, client and driver

**Checkpoint**: the existing source works exactly as before, now on multi-source foundations.

---

## Phase 3: User Story 1 — Browse every configured source at once (P1) 🎯 MVP

**Goal**: More than one source configurable; Discover shows them combined by default with a
dropdown to narrow to one.

**Independent test**: Configure the *same* provider kind twice. Discover interleaves both, the
dropdown lists both plus "All sources", selecting one narrows the list. Needs no second driver.

### Tests

- [ ] T015 [P] [US1] Unit-test round-robin interleaving, uneven page sizes, and source exhaustion in `server/internal/source/merge_test.go`
- [ ] T015a [P] [US1] Test that a title carried by two sources yields two entries and is never de-duplicated (FR-005a), in `server/internal/source/merge_test.go`
- [ ] T015b [P] [US1] Test that single-source mode returns the source's own ordering unpermuted (FR-010), in `server/internal/source/merge_test.go`
- [ ] T016 [P] [US1] Unit-test that one failing or slow source degrades instead of failing the request in `server/internal/source/merge_test.go`
- [ ] T017 [P] [US1] Test source-qualified title id parsing, including ids containing slashes, and rejection of a provider the caller has not configured, in `server/internal/api/source_handlers_test.go`
- [ ] T018 [P] [US1] Test `/v1/source/providers` CRUD including that no response ever echoes session material, in `server/internal/api/source_handlers_test.go`

- [ ] T016a [P] [US1] Test the 8s per-source timeout, the failing-source cooling-off period, and that one query causes at most one page fetch per source (FR-030..FR-032, SC-011), in `server/internal/source/merge_test.go`
- [ ] T017a [P] [US1] Test that a crafted title id cannot change the target host, escape the source's site, or address an unconfigured source, and that a malformed id is a client error (FR-033, FR-034), in `server/internal/api/source_handlers_test.go`
- [ ] T011a [P] Test that unreadable sealed session material is retained and reported rather than discarded, and that nothing decrypted is written during migration (FR-035), in `server/internal/store/source_repos_test.go`

### Implementation

- [ ] T019 [US1] Implement concurrent per-source fan-out with an 8s per-source timeout, failing-source cooling-off, round-robin interleave and degradation reporting in `server/internal/source/merge.go`
- [ ] T020 [US1] Add `SourceID` / `SourceName` to `CatalogTitle` and `Degraded` to `SearchResult` in `server/internal/source/source.go`
- [ ] T021 [US1] Add the `/v1/source/providers` CRUD routes in `server/internal/api/router.go`
- [ ] T022 [US1] Implement the providers CRUD handlers, verifying before persisting, in `server/internal/api/source_handlers.go`
- [ ] T023 [US1] Make search, title and send source-aware with qualified ids, retaining the 0005 routes against the lowest-id source for compatibility, in `server/internal/api/source_handlers.go`
- [ ] T024 [US1] Persist and validate `selectedSource` in the view-state handlers, normalizing an unknown source to "all", in `server/internal/api/source_handlers.go`
- [ ] T025 [P] [US1] Add source-selection and degraded state to `src/composables/useSourceCatalog.ts`
- [ ] T026 [P] [US1] Extend the client API types and calls for qualified ids, `source`, and `degraded` in `src/services/api.ts`
- [ ] T027 [US1] Add the source `ion-select` beside the sort control, shown only at two or more sources, in `src/views/tabs/BrowserPage.vue`
- [ ] T028 [US1] Show a per-result source label in combined mode only, in `src/views/tabs/BrowserPage.vue`
- [ ] T029 [US1] Render the non-blocking degraded notice naming the affected source in `src/views/tabs/BrowserPage.vue`
- [ ] T030 [US1] Make the title modal source-aware for qualified ids in `src/components/SourceTitleModal.vue`
- [ ] T031 [US1] Rework the admin panel from editing one source to managing a list, driven by each kind's declared session fields, in `src/components/SourceProviderAdmin.vue`

**Checkpoint**: US1 is independently shippable and demonstrable with one provider kind twice.

---

## Phase 4: User Story 4 — Credential-free dev and test path (P2)

**Goal**: Everything above runs locally and in CI with no credentials and no real sites.

**Independent test**: With no credentials and no access to the real sites, complete the US1 and
US2 journeys against the local stack.

### Tests

- [ ] T032 [P] [US4] Test that the driver host-allowlist override is unavailable in a production build in `server/internal/source/client_test.go`

### Implementation

- [ ] T033 [US4] Add a fake source site serving the JSON shape of the existing source to `server/internal/synomock/`
- [ ] T034 [US4] Add a fake source site serving zarfilm's HTML shape, from the trimmed fixtures, to `server/internal/synomock/`
- [ ] T035 [US4] Add `/__mock/*` controls for logged-out, paywalled, stalled and short-catalog states in `server/internal/synomock/`
- [ ] T036 [US4] Wire the fake source sites into `server/cmd/synomock/main.go` and `make start`
- [ ] T037 [US4] Add the test-only host-allowlist override, impossible to enable in a production build, in `server/internal/source/client.go`
- [ ] T038 [US4] Add an e2e spec covering the combined-sources journey against the mock in `e2e/`
- [ ] T039 [US4] Add e2e coverage for a degraded source and for narrowing to a single source in `e2e/`
- [ ] T040 [P] [US4] Document both dev paths in `specs/0007-multiple-download-sources/quickstart.md` and confirm the steps as written actually work

**Checkpoint**: CI covers the feature end to end without a single credential.

---

## Phase 5: User Story 2 — zarfilm.com provider (P2)

**Goal**: zarfilm configurable, browsable, searchable, and sendable.

**Independent test**: Configure zarfilm, browse, search, open a title, send a quality. Works
against the mock without credentials, and against the real site with a live session.

### Tests

- [ ] T041 [P] [US2] Test archive-listing parsing (title, year, rating, poster, genres, id) from fixtures in `server/internal/source/providers/zarfilm_parse_test.go`
- [ ] T042 [P] [US2] Test movie title-page parsing including group variants, encoder, size, subtitle type and resolution-from-filename, in `server/internal/source/providers/zarfilm_parse_test.go`
- [ ] T043 [P] [US2] Test series parsing into seasons and episodes in `server/internal/source/providers/zarfilm_parse_test.go`
- [ ] T044 [P] [US2] Test `VerifySession` distinguishing logged-in, logged-out and unsubscribed from fixtures in `server/internal/source/providers/zarfilm_test.go`
- [ ] T045 [P] [US2] Test that a parse failure surfaces as a source-level error and never a panic, in `server/internal/source/providers/zarfilm_test.go`
- [ ] T046 [P] [US2] Test that download hosts are matched by domain suffix and that the dns-prefetch host is *not* allowlisted, in `server/internal/source/providers/zarfilm_test.go`

### Implementation

- [ ] T047 [US2] Implement HTML parsing helpers over `x/net/html` in `server/internal/source/providers/zarfilm_parse.go`
- [ ] T048 [US2] Implement `Kind`, `Hosts` and `SessionFields` with the elevated-sensitivity help text in `server/internal/source/providers/zarfilm.go`
- [ ] T049 [US2] Implement `VerifySession` via the inlined logged-in flag, plus the separate unsubscribed probe, in `server/internal/source/providers/zarfilm.go`
- [ ] T050 [US2] Implement `Search` over paginated archive URLs and `/?s=` text search in `server/internal/source/providers/zarfilm.go`
- [ ] T051 [US2] Implement `Parameters` from the site's own genre and sort taxonomies in `server/internal/source/providers/zarfilm.go`
- [ ] T052 [US2] Implement `Title` for movies and series in `server/internal/source/providers/zarfilm.go`
- [ ] T053 [US2] Implement `ResolveDownload` fetching a fresh signed link at send time in `server/internal/source/providers/zarfilm.go`
- [ ] T053a [US2] Test that the send path forwards no source session field, cookie or header to the NAS (FR-023, SC-006), in `server/internal/api/source_handlers_test.go`
- [ ] T054 [US2] Register the driver and add its image host to the proxy allowlist in `server/internal/source/providers/zarfilm.go`
- [ ] T055 [US2] Surface the `unsubscribed` state through to the admin UI in `src/components/SourceProviderAdmin.vue`
- [ ] T056 [P] [US2] Add the opt-in live check that skips without credentials in `server/internal/source/providers/live_test.go`
- [ ] T057 [US2] **Risk 1**: send a real zarfilm link to a real NAS and settle whether signed links are address-bound; if they are, add a distinct operator-facing error and record the finding in `specs/0007-multiple-download-sources/research.md`

**Checkpoint**: two real sources, combined.

---

## Phase 6: User Story 3 — Shared filter facets (P3)

**Goal**: Combined mode offers only filters every enabled source understands.

**Independent test**: With two sources whose facets differ, the combined sheet shows only
shared filters; selecting one source reveals its extras and switching back drops them visibly.

### Tests

- [ ] T058 [P] [US3] Test facet intersection by English slug with normalized-label fallback, and de-duplication of equivalent options, in `server/internal/api/source_handlers_test.go`

### Implementation

- [ ] T059 [US3] Implement facet intersection in the parameters handler in `server/internal/api/source_handlers.go`
- [ ] T060 [US3] Drop unsupported filters visibly when switching back to combined mode in `src/composables/useSourceCatalog.ts`
- [ ] T061 [US3] Reflect the narrowed facet set in the filter sheet in `src/views/tabs/BrowserPage.vue`
- [ ] T062 [US3] **Risk 3**: measure the real intersection across both live sources; if too thin to be useful, add the curated synonym map in `server/internal/api/source_handlers.go`

---

## Phase 7: Polish & cross-cutting

- [ ] T063 [P] Add a test asserting no session value, cookie or signed link appears in any log line or error payload (SC-010) in `server/internal/api/source_handlers_test.go`
- [ ] T064 [P] Measure combined versus single-source first-page latency against SC-005 and record the result
- [ ] T065 Correct the stale server description in `CLAUDE.md:43` per FR-029 — it claims zero third-party dependencies and no database, while the server has required `modernc.org/sqlite` directly since it became stateful; state the real dependency set including `x/net/html`
- [ ] T066 Confirm no constitution edit is needed for FR-029 (the zero-dependency claim lives in `CLAUDE.md:43`, not the constitution) and that the dependency's justification is recorded under the complexity clause in `specs/0007-multiple-download-sources/plan.md`
- [ ] T067 [P] Document adding and refreshing a second source for operators in `docs/`
- [ ] T068 [P] Update the spec `**Status**:` line and run `make roadmap`
- [ ] T069 Run every gate: `npm run build`, `npm run test:unit:coverage`, `go build ./...`, `go vet ./...`, `go test ./...` with no `LIVE_*` set, and `npm run test:e2e`

---

## Dependencies

```
Setup (T001–T003)
    ↓
Foundational (T004–T014)  ← blocks everything; characterization tests come first
    ↓
US1 (T015–T031)  🎯 MVP — independently shippable
    ↓
US4 (T032–T040)  ← unlocks credential-free development of everything after
    ↓
US2 (T041–T057)  ← needs US1's foundation; developed against US4's mock
    ↓
US3 (T058–T062)  ← needs two sources with differing facets to be meaningful
    ↓
Polish (T063–T069)
```

US4 is sequenced before US2 deliberately: building the driver against a mock that already
exists is faster and safer than building it against a live session that expires.

## Parallel opportunities

- **T001–T003** setup, all independent.
- **T004–T006** the three characterization test files.
- **T015–T018** all US1 test tasks, different concerns.
- **T025–T026** client composable and API layer, while server work continues.
- **T041–T046** all zarfilm test tasks, fixture-driven and independent.
- **T063, T064, T067, T068** polish tasks touching different files.

Server and client tasks within a story generally parallelize, since they meet only at the
contract in `contracts/source-api.md`.

## Implementation strategy

**MVP is US1 alone** — multi-source foundation plus combined Discover, demonstrable by
configuring one provider kind twice. It delivers the entire user-visible payoff of the feature
and could ship before zarfilm exists.

Then US4 (credential-free path), because every later task is cheaper and more repeatable once
the mock exists. Then US2 (the driver), then US3 (filters), which is genuinely optional polish.

Two tasks are investigations that could change the work rather than pure implementation, and
are marked as such: **T057** (are signed links address-bound?) and **T062** (is the facet
intersection usable?).

**Total**: 75 tasks — 3 setup, 11 foundational, 19 US1, 9 US4, 18 US2, 5 US3, 7 polish.

T015a, T015b and T053a were added after `/speckit-analyze` found FR-005a, FR-010 and FR-023
relying on behavior that nothing pinned — FR-005a in particular is satisfied by the merge *not*
de-duplicating, which is exactly the kind of thing a later well-meaning change would break.

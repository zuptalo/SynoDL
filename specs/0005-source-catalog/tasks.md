---
description: "Task list for Download-source catalog (spec 0005)"
---

# Tasks: Download-source catalog

**Input**: Design documents from `/specs/0005-source-catalog/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/source-api.md

**Tests**: REQUIRED — the constitution mandates TDD (Principle II). Failing tests precede implementation in
every phase.

**Organization**: By user story (US1–US5) in priority order, after shared Setup + Foundational phases. v1
**send** is movies-only (US3); series/anime are browse/search only (US2).

## Path Conventions

Web app: server `server/internal/...`, client `src/...`, tests co-located (`_test.go`) + `e2e/`.

---

## Phase 1: Setup

- [ ] T001 Review Docker Scout report for the current `zuptalo/synodl` image and record any fix-available CVEs to bump on this branch (per constitution supply-chain rule); note findings in the PR description draft in `specs/0005-source-catalog/quickstart.md` dev notes.
- [ ] T002 Create the server package skeleton `server/internal/source/` with `doc.go` describing the provider abstraction + outbound host-allowlist boundary.

---

## Phase 2: Foundational (blocking prerequisites)

**⚠️ MUST complete before any user story phase.**

### Storage (migration 0005 + repo) — TDD

- [ ] T003 [P] Write failing store tests in `server/internal/store/source_repos_test.go`: session round-trip is sealed (ciphertext ≠ plaintext, `Open` recovers it), provider config projects only non-secret columns, per-user preferred-quality round-trips, and delete cascades secrets.
- [ ] T004 Add append-only migration 0005 in `server/internal/store/schema.go`: tables `source_providers`, `source_provider_secrets`, `source_prefs` (per data-model.md); do not edit shipped migrations.
- [ ] T005 Implement `server/internal/store/source_repos.go`: `GetProvider`, `SaveProviderConfig`, `SaveProviderSession` (Cipher.Seal), `LoadProviderSession` (Cipher.Open, in-process only), `SetProviderState`, `DeleteProvider`, `GetSourcePref`, `SaveSourcePref` — make T003 pass.

### Provider abstraction + HTTP/2 client — TDD

- [ ] T006 [P] Write failing tests in `server/internal/source/client_test.go` against an `httptest` fake provider: request uses HTTP/2-capable transport + browser headers + session cookie/`c-*` headers; a host NOT in the allowlist is refused; a challenge body → `ErrNeedsRefresh{Layer:clearance}`; API 401/unauth → `ErrNeedsRefresh{Layer:token}`.
- [ ] T007 Implement `server/internal/source/errors.go` (typed `ErrNeedsRefresh` with layer, `ErrChallenge`, `ErrIPMismatch`) and `server/internal/source/client.go` (stdlib `net/http` `Transport{ForceAttemptHTTP2:true}`, browser+`c-*` headers, gzip transparent, **host allowlist**, secret-free error mapping) — make T006 pass.
- [ ] T008 [P] Define `server/internal/source/source.go`: `Provider` interface (`VerifySession`, `Search`, `TitleDownloads`) + `kind`→driver registry; transient types `CatalogTitle`, `QualityOption` (data-model.md).

### Provider driver (first provider) — TDD

- [ ] T009 [P] Write failing tests in `server/internal/source/providers/thirtynama_test.go`: parse a captured `advanced_search`/`full_search` fixture → `[]CatalogTitle` (id/type/title/poster/imdb/score/flags + pagination); parse a `download/id` fixture → `[]QualityOption`; `VerifySession` maps a sample-call success/failure correctly. Use redacted JSON fixtures under `providers/testdata/`.
- [ ] T010 Implement `server/internal/source/providers/thirtynama.go`: endpoint mapping (full_search, advanced_search, download/id), request bodies (`query=`, `parameters=<json>`), response parsing → make T009 pass. No secrets in logs.

**Checkpoint**: storage + provider client + first driver exist and are unit-tested behind the host allowlist.

---

## Phase 3: User Story 1 — Admin configures a source provider (P1)

**Goal**: Admin pastes session material; server verifies then stores it encrypted; only non-secret status is
ever returned. **Independent test**: valid material → verified + active; invalid → specific error, nothing
stored; non-admin blocked; reads never leak secrets.

- [ ] T011 [P] [US1] Write failing handler tests in `server/internal/api/source_handlers_test.go` (config subset): `PUT /v1/source/session` requires admin, verifies via a fake provider before storing, stores nothing on failure; `DELETE` resets to `not_configured`; `GET /v1/source/status` returns non-secret status incl. `canManage`; assert **no secret string** appears in any response body or in captured log output (redaction test).
- [ ] T012 [US1] Implement the config handlers in `server/internal/api/source_handlers.go`: `GET /v1/source/status` (requireUser), `PUT /v1/source/session` + `DELETE /v1/source/session` (requireAdmin), wiring store + `source` verify; map verify failures to `provider_verify_failed{reason}` without echoing values.
- [ ] T013 [US1] Register the US1 routes under the `if d.Stateful` block in `server/internal/api/router.go`; confirm they are absent in legacy mode.
- [ ] T014 [P] [US1] Client: add `getSourceStatus`, `putSourceSession`, `deleteSourceSession` to `src/services/api.ts` with types (no secret ever returned).
- [ ] T015 [P] [US1] Client: build `src/components/SourceProviderAdmin.vue` (stock Ionic form: kind, movies/tv parent, paste fields, Verify & Save, status chip, Refresh, Remove) and mount it for admins in `src/views/tabs/SettingsPage.vue`.

**Checkpoint**: an admin can configure/refresh/remove a provider; secrets are write-only end to end.

---

## Phase 4: User Story 2 — Browse and search the catalog (P1)

**Goal**: Signed-in users browse/search with provider filters; unavailable/needs-refresh states render
cleanly. **Independent test**: active provider → results with posters/ratings; filters narrow; expired →
refresh state; unconfigured → empty state.

- [ ] T016 [P] [US2] Write failing handler tests in `server/internal/api/source_handlers_test.go` (search subset): `POST /v1/source/search` requires user, passes filters through the fake provider, returns items+pagination; expired session → HTTP 409 `source_needs_refresh{layer}`; unconfigured/legacy → clean unavailable signal.
- [ ] T017 [US2] Implement `POST /v1/source/search` in `source_handlers.go` (requireUser) + register route; translate `ErrNeedsRefresh`→409, persist `state=needs_refresh` + surface layer.
- [ ] T018 [P] [US2] Client: add `searchSource(query, filters, page)` to `src/services/api.ts`; add `src/composables/useSourceCatalog.ts` (results, pagination, loading, needs-refresh/unavailable state).
- [ ] T019 [US2] Client: replace the placeholder `src/views/tabs/BrowserPage.vue` with the catalog — `ion-searchbar`, results grid (`ion-card`/`ion-img` poster + title + scores), infinite/`ion-infinite-scroll` pagination, and the unavailable/needs-refresh empty states (admin sees a configure/refresh CTA linking to Settings).
- [ ] T020 [P] [US2] Client: build `src/components/SourceFilterSheet.vue` (stock Ionic `ion-select`/`ion-chip` for type/quality/genre/language/country) wired into `useSourceCatalog`.
- [ ] T021 [P] [US2] Add a vitest unit for any pure helper (e.g. filter-param serialization / title normalization) in `src/services/` and keep it on the coverage allowlist.

**Checkpoint**: the Browser tab is a working, filterable catalog with correct empty/refresh states.

---

## Phase 5: User Story 3 — Pick a quality and send it to the NAS (P1, movies)

**Goal**: Open a movie → see qualities → pick → Send to NAS → per-title subfolder under movies parent + task
added, honoring folder grants. **Independent test**: released movie → pick quality → Send → subfolder created
+ task targets it; non-admin outside grants refused; existing subfolder reused.

- [ ] T022 [P] [US3] Write failing handler tests in `server/internal/api/source_handlers_test.go` (title+send subset): `GET /v1/source/title/{id}` returns qualities for a movie and `sendable:false` for series/anime; `POST /v1/source/send` resolves a signed link via fake provider, validates `dest` with `authz.AllowedForCreate` (non-admin outside grants → 403 `destination_forbidden`), reuses an existing subfolder, calls fake `syno.Client` CreateFolder + CreateTaskURIs; link/host/NAS failure → 502 `send_failed{reason}`; expired → 409; assert the signed URL never appears in logs.
- [ ] T023 [US3] Implement `GET /v1/source/title/{id}` (requireUser) in `source_handlers.go`: fetch title downloads via provider; movies list qualities, series/anime return `sendable:false`.
- [ ] T024 [US3] Implement `POST /v1/source/send` (requireUser): resolve signed link at send time (never cached), compute `dest = <moviesParent>/<sanitized title>`, `authz.AllowedForCreate` gate, `CreateFolder` (reuse if exists) + `CreateTaskURIs([link],{destination})`; map failures per contract; register both routes.
- [ ] T025 [P] [US3] Client: add `getSourceTitle(id)` and `sendSource(titleId, qualityId)` to `src/services/api.ts`.
- [ ] T026 [US3] Client: build `src/components/SourceTitleModal.vue` (stock Ionic modal: poster/meta, quality list `ion-radio`/`ion-list`, Send to NAS button, per-user preferred-quality preselect hook, disabled state for non-sendable) and open it from `BrowserPage.vue`.
- [ ] T027 [US3] Client: on send success, surface an `ion-toast` and ensure the new task shows in the Tasks tab (reuse existing tasks refresh); handle 403/409/502 with clear messages.

**Checkpoint**: end-to-end movie send works; MVP is now complete (US1+US2+US3).

---

## Phase 6: User Story 4 — Session expires and admin refreshes (P2)

**Goal**: Expiry surfaces a clear "needs refreshing" state to users and an actionable refresh to admins; no
secret exposure. **Independent test**: invalidate session → users see refresh state, admin re-pastes → resumes.

- [ ] T028 [P] [US4] Write failing tests: server maps clearance-vs-token-vs-IP failures to `source_needs_refresh{layer}` and flips `state=needs_refresh` (extend `source_handlers_test.go`); a re-`PUT` of valid material returns `state=active`.
- [ ] T029 [US4] Implement state transitions in handlers/store (`active`→`needs_refresh` on failure, `→active` on successful verify/refresh); ensure IP-mismatch (FR-019) maps to needs-refresh, not a hang/timeout leak.
- [ ] T030 [P] [US4] Client: `useSourceCatalog` + `BrowserPage.vue` render the "source needs refreshing" state for users and a refresh CTA for admins; `SourceProviderAdmin.vue` shows `needs_refresh` and supports one-step re-paste.

**Checkpoint**: expiry is graceful and self-serve for the admin.

---

## Phase 7: User Story 5 — Preferred quality (P3)

**Goal**: Per-user preferred quality auto-selects when offered. **Independent test**: set pref → title offering
it preselects; title lacking it → manual pick.

- [ ] T031 [P] [US5] Write failing tests: `GET/PUT /v1/source/prefs` (requireUser) round-trip + normalization (`source_handlers_test.go`).
- [ ] T032 [US5] Implement `GET/PUT /v1/source/prefs` handlers + routes using `source_repos` pref methods.
- [ ] T033 [P] [US5] Client: add `getSourcePrefs`/`setSourcePrefs` to `api.ts`; a small preferred-quality setting in `SourceProviderAdmin.vue` or user settings; preselect logic in `SourceTitleModal.vue` (exact/normalized match, fallback to manual).

**Checkpoint**: frequent users skip the picker when possible.

---

## Phase 8: Polish & Cross-Cutting

- [ ] T034 [P] Add a mock provider double for e2e (endpoint in `server/cmd/synomock` or a test fixture server) so the Browser flow runs with no real provider/NAS (mock-DSM parity rule).
- [ ] T035 [US2] [US3] Add a Playwright e2e in `e2e/` covering: admin configures provider (mock) → user searches → opens a movie → sends → task appears; plus the needs-refresh state.
- [ ] T036 [P] Redaction audit: grep/log-capture test proving session material, `c-token`, cookies, and full signed URLs never appear in logs/errors/metrics across handlers (Principle III).
- [ ] T037 [P] Docs: finalize `quickstart.md`; update `CLAUDE.md`/`README` notes for the new `internal/source` package + off-by-default provider config; note the outbound-host allowlist.
- [ ] T038 Run the checklist gate (`/speckit-checklist`) for the credential boundary + outbound allowlist and resolve any findings before implement sign-off.
- [ ] T039 Full gate run: `npm run build`, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run test:unit:coverage`, `npm run test:e2e`; ensure coverage floors hold or ratchet.

---

## Dependencies & completion order

- **Setup (P1: T001–T002)** → **Foundational (P2: T003–T010)** block everything.
- **US1 (T011–T015)** enables all others (nothing works without a configured provider) — do first.
- **US2 (T016–T021)** depends on Foundational + US1.
- **US3 (T022–T027)** depends on Foundational + US1 (and US2 for the UI entry point).
- **US4 (T028–T030)** builds on US1/US2 error paths.
- **US5 (T031–T033)** independent once Foundational + US1 exist; touches US3 modal for preselect.
- **Polish (T034–T039)** last; T038 checklist gate MUST pass before implement is considered done.

## Parallel opportunities

- Foundational: T003/T006/T008/T009 are `[P]` (different files) — tests can be authored together.
- Within each story, `[P]` client `api.ts` + component tasks run alongside server tasks (different files).
- US5 can proceed in parallel with US4.

## MVP scope

**US1 + US2 + US3** (movies end-to-end). US4 (expiry UX) and US5 (preferred quality) are incremental
follow-ons on the same branch.

## Format validation

All tasks use `- [ ] Txxx [P?] [USx?] description + file path`; Setup/Foundational/Polish carry no story
label; user-story tasks carry theirs.


## Reconciled 2026-09-04

The spec is marked shipped because the feature is implemented and released —
verified against the code, not against these boxes. The checkboxes were never
maintained during implementation and are left as written: ticking them now would
claim each task was completed as specified, which is more than was checked.

# Implementation Plan: Download-source catalog

**Branch**: `feat/0005-source-catalog` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0005-source-catalog/spec.md`

## Summary

Turn the placeholder **Browser** tab into a real catalog backed by an admin-configured external "source
provider". The Go server stores the provider's session material (encrypted, write-only) and calls the
provider's JSON API server-side over HTTP/2 so signed-in users can browse, search, and quality-filter the
catalog, then send a chosen movie quality to Download Station — landing it in a per-title subfolder under the
admin-designated movies parent, reusing the existing folder-create + task-create flow and per-user folder
grants. Feasibility (Cloudflare bypass with pure Go stdlib + HTTP/2, authenticated API, signed self-
authenticating download links fetchable by DSM from the shared public IP) was verified end-to-end before this
plan. v1 scopes **send** to movies; series/anime are browse/search only.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript / Vue 3 + Ionic (client), Node 22 build.

**Primary Dependencies**: Server — stdlib `net/http` only (HTTP/2 is built in via `ForceAttemptHTTP2`; **no
new third-party dependency**), existing `internal/store` (SQLite `modernc.org/sqlite` + `Cipher`),
`internal/authz`, `internal/nas`, `internal/syno`. Client — existing Ionic component set, `useLiveQuery`,
`api.ts`.

**Storage**: The single existing SQLite volume. One new append-only migration adds provider config +
encrypted session + per-user preferred-quality. No second datastore; catalog results are **not** persisted
(fetched live, like tasks).

**Testing**: Go table tests against a fake provider `http` server (`httptest`) and the existing fake
`syno.Client`; vitest for pure client modules; e2e (Playwright) for the Browser-tab flow against a new mock
provider endpoint in `cmd/synomock` (or a dedicated test double). TDD: failing tests precede implementation.

**Target Platform**: Single container (`synodl`) serving PWA + `/v1`; installable PWA on mobile.

**Project Type**: Web application (Go backend + Vue/Ionic PWA), stateful mode (`SECRETS_KEY` set).

**Performance Goals**: Catalog search/detail responses feel instant to the user (dominated by the upstream
provider round-trip; add negligible server overhead). Send-to-NAS completes within a normal task-create.

**Constraints**: Pure-Go/stdlib, no headless browser, no third-party TLS lib. Outbound limited to the
configured provider hosts. Secrets encrypted at rest, never returned/logged. Same stable public IP as the NAS.

**Scale/Scope**: Self-hosted, small user counts (household/team). One active provider in v1; abstraction
allows more later.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven Development** — ✅ Numbered planned spec `0005`; pipeline followed (specify → clarify
  (decisions locked) → plan → tasks → analyze → checklist → implement). Traceable branch/issues/PR.
- **II. Test-Driven Development** — ✅ Plan mandates failing tests first: provider client (httptest fake),
  handlers (fake syno + fake provider), store round-trip + encryption, authz on send destination, and a
  client-flow e2e. Server coverage-floored packages untouched or ratcheted up.
- **III. Custodial State & Credential Safety (NON-NEGOTIABLE)** — ⚠️ **This is the load-bearing gate.**
  - *One store, one volume*: ✅ new state is one SQLite migration; no new datastore; catalog data not
    persisted.
  - *Secrets encrypted at rest*: ✅ provider session material stored only as a `Cipher`-sealed blob under
    `SECRETS_KEY`, same as the NAS password.
  - *Credentials never leak*: ✅ session material is write-only (never returned to clients), never logged,
    never placed in URLs/query strings; only non-secret status is exposed.
  - *DSM allowlist still governs the NAS*: ✅ no change to `internal/syno`'s NAS allowlist; the NAS is still
    reached only through existing typed calls.
  - *New outbound target*: ⚠️ This feature adds outbound calls to the **configured provider hosts** — a
    deliberate widening beyond the NAS. It is **operator-opt-in, OFF by default**, and bounded to the hosts
    declared in the provider config (never client-supplied targets, never an open proxy). Documented in the
    **Credential-Safety Impact** section below; requires a **`/speckit-checklist`** pass before implement.
  - *Least privilege*: ✅ send-to-NAS destination is validated against per-user folder grants
    (`authz.AllowedForCreate`), same as task-create.
- **IV. Offline-First Client Data** — ✅ Only the per-user preferred-quality may cache client-side via
  existing IndexedDB patterns (server is source of truth); catalog listings are live, not persisted. Any new
  object store bumps `DB_VERSION`.
- **V. Quality Gates** — ✅ `npm run build`, `go build/vet/test`, vitest+floors, e2e-where-changed all in the
  Definition of Done. User-facing commit subjects written as release-note copy.
- **VI. Ionic-First UI** — ✅ Browser tab, filters, title sheet, quality picker, and admin config built from
  stock Ionic (`ion-searchbar`, `ion-chip`/`ion-select` filters, `ion-card`/`ion-img` result grid,
  `ion-modal`, `ion-list`, `ion-fab` reuse) + existing `--app-*` tokens. No bespoke widgets.
- **VII. Traceable Delivery** — ✅ Tasks → issues; PR lists `Closes #N`; ROADMAP generated. Off-by-default so
  the merge is safe to publish.

**Result**: PASS with one flagged widening (new outbound target) that is justified, bounded, opt-in, and
routed through the required checklist gate. No unjustified violations → proceed to Phase 0.

## Credential-Safety Impact *(Principle III, mandatory)*

- **What is stored & how protected**: Per provider, a JSON blob of session material — Cloudflare clearance
  cookie, app API key, per-user auth token, User-Agent — sealed with the existing AES-256-GCM `Cipher`
  (`SECRETS_KEY`-derived) as `source_provider_secrets.session_enc`. Non-secret config/status
  (`source_providers`: kind, hosts, parents, state, last_verified_at) is plaintext. Per-user preferred
  quality is non-secret.
- **What crosses to the provider**: The stored headers/cookie ride each server→provider HTTPS/2 request.
  Nothing user-identifying beyond what the provider already issued the admin.
- **What crosses to the NAS**: Only a signed, self-authenticating download URL handed to Download Station via
  the existing `CreateTaskURIs` path + a destination folder. No provider secrets reach the NAS.
- **What could appear in logs/errors**: Route + outcome only. Session material, the `c-token`, cookies, and
  full signed download URLs MUST be redacted from logs, errors, and metrics. Verification failures report a
  category ("invalid/expired session", "challenge") without echoing values.
- **Why**: Mirrors the shipped NAS-password treatment; keeps the never-leak guarantee while enabling the
  catalog.

## Project Structure

### Documentation (this feature)

```text
specs/0005-source-catalog/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (endpoint contracts)
│   └── source-api.md
├── checklists/
│   └── requirements.md  # spec-quality checklist (done)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
server/
├── internal/
│   ├── source/                     # NEW: generic source-provider abstraction
│   │   ├── source.go               #   Provider interface + registry (kind → driver)
│   │   ├── client.go               #   HTTP/2 stdlib client (browser headers + session), host-allowlisted
│   │   ├── errors.go               #   typed errors: ErrNeedsRefresh, ErrChallenge, ErrIPMismatch
│   │   └── providers/
│   │       └── thirtynama.go       #   first concrete provider (endpoint mapping + response parsing)
│   ├── store/
│   │   ├── schema.go               # +migration 0005: source_providers, source_provider_secrets, source_prefs
│   │   └── source_repos.go         # NEW: get/save provider config+secrets (sealed), per-user pref
│   └── api/
│       ├── source_handlers.go      # NEW: status, admin session PUT/DELETE, search, title, send, prefs
│       └── router.go               # register new routes under the existing `if d.Stateful` block
│
src/                                # client (PWA)
├── views/tabs/BrowserPage.vue      # REPLACE placeholder with the catalog
├── components/
│   ├── SourceTitleModal.vue        # NEW: title detail + quality picker + Send to NAS
│   ├── SourceFilterSheet.vue       # NEW: type/quality/genre/language/country filters (Ionic)
│   └── SourceProviderAdmin.vue     # NEW: admin config in Settings (paste/verify/refresh/remove)
├── composables/useSourceCatalog.ts # NEW: search/detail/send state; needs-refresh handling
├── services/api.ts                 # +source endpoints
└── views/tabs/SettingsPage.vue     # mount SourceProviderAdmin for admins

e2e/                                # Browser-tab flow against a mock provider
```

**Structure Decision**: Web-application layout (existing). The provider integration is a **new self-contained
server package** `internal/source` (interface + stdlib HTTP/2 client + per-provider driver) so the core stays
provider-neutral and the outbound surface is isolated and testable against an `httptest` fake. Persistence is
one new migration + repo in the existing `store`. Client work replaces the Browser placeholder and adds an
admin config surface in Settings, all stock Ionic.

## Phased approach (for /speckit-tasks)

- **Phase A — Provider abstraction + client (server, TDD)**: `internal/source` interface, stdlib HTTP/2 client
  with browser headers + host allowlist, typed errors, `thirtynama` driver (search/advanced_search, title
  download for movies). Tests against an `httptest` fake provider (success, challenge, expired-token→needs-
  refresh, IP-mismatch).
- **Phase B — Storage (server, TDD)**: migration 0005 + `source_repos.go` (sealed session round-trip, config,
  per-user pref). Store tests incl. encryption + write-only.
- **Phase C — Handlers + routes (server, TDD)**: status (user), session PUT/DELETE (admin, verify-before-
  store), search (user), title (user), send (user; subfolder create + task-create + folder-grant check),
  prefs (user). Handler tests with fake `syno.Client` + fake provider; redaction assertions.
- **Phase D — Client catalog (PWA)**: Browser tab grid + searchbar + filter sheet, title modal + quality
  picker + Send to NAS, needs-refresh/empty states, `useSourceCatalog`, `api.ts` additions.
- **Phase E — Admin config (PWA)**: `SourceProviderAdmin` in Settings (paste/verify/refresh/remove, status).
- **Phase F — e2e + docs**: Playwright happy-path against a mock provider; quickstart; supply-chain scan note.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New outbound target beyond the NAS (provider hosts) | The catalog cannot exist without server-side calls to the provider; browser-side is blocked by CORS/X-Frame-Options and would expose credentials | Client-side calls rejected (credential exposure + CORS); an open proxy rejected (violates allowlist) — access is bounded to configured hosts, opt-in, off by default |

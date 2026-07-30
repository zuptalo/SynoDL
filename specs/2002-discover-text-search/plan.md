# Implementation Plan: Fix Discover text search filtering

**Branch**: `fix/2002-discover-text-search` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/2002-discover-text-search/spec.md`

## Summary

Two defects in the Discover text-search path, both verified live against the 30nama API:

1. **Type filter breaks search.** `thirtynama.Search` builds `full_search/type/{code}` from the
   numeric type code (`15`/`16`/`17`). `full_search` only accepts `type/all`; a numeric code (or
   the slug) returns `success:true` with **zero** posts. Fix: `full_search` always uses `type/all`,
   and the driver re-filters the returned titles by `title_type` when a type filter is set, so the
   Type filter keeps working during search.
2. **Sort + facet filters silently do nothing during search.** `full_search` cannot sort or
   facet-filter (confirmed: the provider's own `search` endpoint returns identical results for
   `orderby/imdb` vs `orderby/relevant` and ignores a `parameters` body). Fix (client, honesty
   only): while a query is active, disable the sort control and the non-type facet filters with a
   short hint; the Type filter stays enabled.

Browse (empty query → `advanced_search`) is already correct and is not touched.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript / Vue 3 + Ionic (client)

**Primary Dependencies**: stdlib `net/http` (server, zero third-party); Vite/Vitest, Playwright (e2e)

**Storage**: none added — no new persisted state (Principle III preserved)

**Testing**: `go test ./...` (fake `syno`/source client), `vitest` unit, Playwright e2e

**Target Platform**: single container (synodl serves the PWA + `/v1` API); deployed to k3s via Keel

**Project Type**: web application (Go backend + Vue PWA frontend, one repo)

**Performance Goals**: no change; type re-filter is an in-memory pass over one page of results

**Constraints**: stateless credential-free proxy; DSM/source allowlist unchanged; no secrets in logs

**Scale/Scope**: ~2 files changed server-side, ~2 client files; unit + integration test coverage

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven Development** — ✅ this spec (`2002`) drives the change; full pipeline followed.
- **III. Custodial State & Credential Safety (NON-NEGOTIABLE)** — ✅ no new state, no credentials
  touched, no logging of session/URIs. The change is behaviour inside the existing
  `internal/source` driver; it adds **no** new source endpoint and stays within the current host
  allowlist (`full_search`, `advanced_search`, and `advanced_search_parametres` are already used).
- **TDD mandate** — ✅ server change is covered by table tests against the fake source client
  (assert `type/all` path + type re-filter); client change covered by unit tests on the
  "query active ⇒ controls disabled" derivation.
- **V. Release-note subject** — ✅ user-facing `fix(...)` subject in plain language.
- **Checklist gate** (credential boundary / DSM allowlist) — **not triggered**: no allowlist or
  credential-boundary surface changes, so a `/speckit-checklist` run is not required.

No violations → no Complexity Tracking entries.

## Project Structure

### Documentation (this feature)

```text
specs/2002-discover-text-search/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0 output
├── quickstart.md        # Phase 1 output (manual verification)
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

Data model and API contracts: **N/A** — no new entities and the `/v1/source/search` request/response
shape is unchanged. Only internal driver behaviour and client control-state derivation change.

### Source Code (repository root)

```text
server/internal/source/providers/
├── thirtynama.go        # Search(): full_search always type/all; re-filter posts by title_type
└── thirtynama_test.go   # assert path == type/all; assert type re-filter drops non-matching

src/
├── composables/useSourceCatalog.ts   # expose `searchActive` / disabled-control derivation
├── services/source-filters.ts        # (only if a shared helper is cleaner)
└── views/tabs/…Browser view + filter sheet + sort control  # disable + hint when query active
```

**Structure Decision**: Web-app layout (existing). Server fix lives in the one provider file that
owns the 30nama path building; the client honesty fix lives in `useSourceCatalog` (single source of
truth for query/sort/filter state) plus the Browser view + filter sheet that render the controls.

## Complexity Tracking

No constitution violations — section intentionally empty.

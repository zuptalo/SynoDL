# Implementation Plan: Multiple Download Sources

**Branch**: `feat/0007-multiple-download-sources` | **Date**: 2026-09-03 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/0007-multiple-download-sources/spec.md`

## Summary

Lift the source-catalog layer from one configured provider to many, add zarfilm.com as a second
provider driver, and show all sources combined in Discover with a dropdown to narrow to one.

The work divides cleanly. The **foundation** (US1) is a refactor: the store's singleton
accessors become list accessors, `/v1/source/*` becomes source-aware, title ids gain a source
prefix, and the API learns to fan out and interleave. The **driver** (US2) is new code behind
an interface that already exists, plus its first HTML-parsing dependency. The **dev/test
path** (US4) extends the existing mock so none of this needs credentials to run. **Filters**
(US3) is the smallest piece, sitting on top of facet data the drivers already report.

The refactor is the risky half, not the new site: it touches code paths that currently work,
and its failure mode is breaking the existing source for existing users. Every step of it is
therefore covered by tests written against the current behavior first.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: Server — standard library, plus **`golang.org/x/net/html`, the first
third-party server dependency** (R6/FR-029). The zero-dependency claim lives in `CLAUDE.md:43`
and nowhere else — the constitution has no such Domain Constraint, only a general
complexity-justification clause that a new dependency triggers, answered by the Complexity
Tracking table below. Client — existing Ionic/Vue stack, no new dependencies.

**Storage**: SQLite, single volume. Three additive `ALTER TABLE` migrations; no table is
created and none is rewritten.

**Testing**: `go test` against fakes and `httptest` (server), Vitest (client pure modules),
Playwright (e2e, hermetic against `synomock`), plus opt-in live checks against the real sites
that skip without credentials.

**Target Platform**: Single container serving the PWA and API on one origin.

**Project Type**: Web application — Vue PWA + Go service in one repo, one image.

**Performance Goals**: Combined Discover first page within 150% of single-source today
(SC-005), achieved by fetching sources concurrently rather than sequentially.

**Constraints**: Custody rules of Principle III — sessions sealed at rest, write-only, never
logged, never sent to the NAS. Outbound calls confined to per-driver host allowlists. Signed
download links treated as secrets (they embed the account id).

**Scale/Scope**: Single-digit sources. ~2 new server packages' worth of driver code, one store
refactor, one API fan-out layer, one new client control plus per-item source labelling.

## Constitution Check

| Principle | Status | Notes |
|---|---|---|
| I. Spec-Driven Development | **Pass** | Spec → clarify → this plan; no code before tasks |
| II. Test-Driven Development | **Pass** | Refactor steps get characterization tests against current behavior first; driver parsing is fixture-driven from real captures; merge logic is unit-tested before wiring |
| III. Custodial State & Credential Safety | **Pass, with a required disclosure** | Session shape changes but custody does not: still sealed, still write-only. The *new* obligations this feature creates are (a) a bigger blast radius for zarfilm's cookie, disclosed at the paste point, and (b) signed links must be treated as secrets in logs because they embed the account id. Spec carries a full Credential-Safety Impact section |
| IV. Offline-First Client Data | **Pass** | No new IndexedDB store; the source selection lives server-side with the rest of the view state |
| V. Quality Gates Are the Definition of Done | **Pass** | All five gates run; the new dependency must pass `go mod tidy` cleanly and be vendored in the image build |
| VI. Ionic-First UI | **Pass** | The selector is an `ion-select` matching the existing sort control; no bespoke widget |
| VII. Traceable, Auto-Closing Delivery | **Pass** | Tasks become issues; the PR lists `Closes #N` for each |

**Project property amended, not violated**: "zero third-party dependencies" is a stated
project property being deliberately changed by FR-029, with the reasoning recorded in R6. It is
asserted in `CLAUDE.md:43` only — **not** in the constitution, which instead requires that any
added dependency be justified (`constitution.md:250`), satisfied by the Complexity Tracking
table below. So this needs one documentation edit, not a constitution amendment; flagged here
so it is neither mistaken for drift nor over-corrected into a governance change it does not
require.

**Mock-DSM dev parity**: extended in spirit — sources become the second outbound target the
mock covers, so `make start` and e2e still need no real hardware and now no real credentials
either.

## Project Structure

### Documentation (this feature)

```
specs/0007-multiple-download-sources/
├── spec.md
├── plan.md              # this file
├── research.md          # Phase 0 — decisions + the live zarfilm findings
├── data-model.md        # Phase 1 — migrations, entity/wire shapes, state machine
├── contracts/
│   └── source-api.md    # Phase 1 — amended /v1/source contract
├── quickstart.md        # Phase 1 — how to run both sources locally
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```
server/
├── internal/store/
│   ├── schema.go              # +3 migrations (sort_order, last_error, selected_source)
│   └── source_repos.go        # singleton accessors → list accessors; session bag migration
├── internal/source/
│   ├── source.go              # Session reshape, SessionField, SourceID on titles, Degraded
│   ├── client.go              # stop hardcoding one provider's auth headers
│   ├── merge.go               # NEW — round-robin interleave, concurrent fan-out, degradation
│   └── providers/
│       ├── thirtynama.go      # adapt to the new Session shape; behavior unchanged
│       ├── zarfilm.go         # NEW — the driver
│       ├── zarfilm_parse.go   # NEW — HTML → catalog structs
│       └── testdata/zarfilm/  # NEW — trimmed captures of real responses
├── internal/synomock/         # NEW fake source sites (JSON + HTML shapes)
└── internal/api/
    ├── router.go              # +/v1/source/providers routes
    └── source_handlers.go     # source-aware; fan-out; qualified ids

src/
├── composables/useSourceCatalog.ts   # selected source, degraded state
├── views/tabs/BrowserPage.vue        # source ion-select beside the sort control
├── components/SourceProviderAdmin.vue# one source → a list of sources
└── components/SourceTitleModal.vue   # source-aware title ids

e2e/                                  # combined-sources journey against the mock
```

**Structure Decision**: No new top-level structure. The feature lands in the existing
`internal/source` (drivers + merge), `internal/api` (routing), `internal/store` (persistence)
and the existing Discover client files. `merge.go` is new because interleaving is neither a
driver's job nor a handler's — it is the one genuinely new concept this feature introduces.

## Implementation phases

Ordered so that each phase leaves the tree green and the existing source working.

**Phase A — characterize before refactoring.** Add tests that pin the *current* single-source
behavior end to end. These are the safety net for everything after, and they must pass
unchanged once the refactor lands.

**Phase B — store: singleton to list.** Migrations, list accessors, the sealed-session bag
migration (old blobs read losslessly). Existing single-row installations must upgrade untouched
— tested explicitly, since silently stranding an operator's pasted session is the worst
failure mode here.

**Phase C — session reshape.** `Session` becomes a declared bag; `Client.Do` stops knowing any
provider's header names; `thirtynama` moves its own auth into itself with no behavior change.

**Phase D — API: source-aware routes.** Qualified title ids, `/providers` CRUD, per-request
provider validation, retained 0005 routes addressing the lowest-id source for compatibility.

**Phase E — fan-out and merge.** `merge.go`: concurrent per-source fetch with timeouts,
round-robin interleave, degradation reporting. Unit-tested against fake providers before it is
wired to anything real.

**Phase F — mock sources + e2e.** The credential-free path (US4), which everything after can
be developed against.

**Phase G — zarfilm driver.** Parsing from real captures, all six `Provider` methods, the
unsubscribed-state probe, live checks.

**Phase H — client.** Source selector, per-item source labels, degraded notice, admin list UI.

**Phase I — filters.** Facet intersection in combined mode (US3).

**Phase J — governance + docs.** FR-029 amendments to `CLAUDE.md` and the constitution,
operator documentation for adding a second source.

## Complexity Tracking

| Decision | Why it is worth the complexity | Simpler alternative rejected |
|---|---|---|
| First third-party server dependency | zarfilm serves no structured data; regex over nested markup ships subtle parse bugs | Standard library only — considered and rejected by the user (R6) |
| New `merge.go` layer | Interleaving belongs to neither a driver nor a handler | Merging inside a handler — would entangle fan-out with HTTP concerns |
| Session becomes a declared bag | Two sites authenticate incompatibly; also stops each driver seeing the others' secrets | Adding a `Cookie` field — grows a field per site forever |
| Source-qualified title ids | Combined lists must hand a title back unambiguously | A parallel `?source=` param — id and source could be recombined wrongly |
| Mock source sites in `synomock` | The only route to CI coverage; e2e boots stateless and cannot hold a session | Fixture replay alone — cannot exercise client, merge, or UI |

## Risks

Carried from `research.md`, with the two that could change the shape of the work:

1. **Signed links may be address-bound.** Unverified — the portability check ran from the same
   address that minted the link. Must be settled early against a real NAS (Phase G). If
   confirmed, split SynoDL/NAS deployments cannot download from this source, and that needs a
   distinct operator-facing error rather than a generic failure.
2. **Facet intersection may be uselessly thin.** Measurable only once both sources are live.
   Fallback is a curated synonym map in the API layer; noted, not built.

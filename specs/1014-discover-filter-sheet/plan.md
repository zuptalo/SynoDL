# Implementation Plan: Discover filter sheet polish

**Branch**: `feat/1014-discover-filter-sheet` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/1014-discover-filter-sheet/spec.md`

## Summary

Presentation-only follow-up to spec 2002. Client changes only; no server, no API, no result-set
behaviour change.

- **BrowserPage.vue**: move the search hint from the fixed `<ion-header>` into the scrolling
  `<ion-content>` (with the active-filter chips); reword it without a dash and make the "clear the
  search…" nudge conditional on an ineffective selection existing; strike-through/dim the non-type
  chips and the sort control while `searchActive`.
- **SourceFilterSheet.vue**: switch Type and Min-rating selects from `interface="popover"` to
  `interface="alert"`; regroup controls into titled sections (`ion-list inset` + `ion-list-header`)
  mirroring the Settings screen.
- **useSourceCatalog.ts**: add a derived `searchIneffective` (searchActive AND at least one non-type
  filter or a non-default sort/order) to drive the conditional hint nudge, with a unit test.

## Technical Context

**Language/Version**: TypeScript / Vue 3 + Ionic (client only)

**Primary Dependencies**: Ionic Vue (`ion-select` `interface`, `ion-chip`, `ion-list-header`, `::part`)

**Storage**: none — no persisted state changes

**Testing**: vitest unit (pure derivation); manual on device / deployed build for the visual bits

**Target Platform**: PWA; k3s via Keel (same as 2002)

**Project Type**: web application (client change only)

**Constraints**: theme-aware (light+dark) strikethrough/dim; stateless/credential boundary untouched

**Scale/Scope**: 3 client files, 1 new derivation + test

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Spec-Driven Development** — ✅ spec `1014` drives it; pipeline followed.
- **III. Custodial State & Credential Safety** — ✅ presentation only; no state, no credentials, no
  source API touched. Checklist gate not triggered.
- **TDD** — ✅ the one piece of logic (`searchIneffective`) gets a unit test; the rest is markup/CSS
  verified visually (no meaningful pure logic to gate).
- **V. Release-note subject** — ✅ user-facing `fix`/`feat(browser)` plain-language subject.

No violations → no Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/1014-discover-filter-sheet/
├── spec.md
├── plan.md
├── checklists/requirements.md
└── tasks.md
```

Research / data-model / contracts: N/A (UI polish, no new decisions, entities, or contracts).

### Source Code (repository root)

```text
src/composables/useSourceCatalog.ts   # + searchIneffective derivation (and test)
src/composables/useSourceCatalog.test.ts
src/views/tabs/BrowserPage.vue        # hint move + reword + chip/sort ineffective marking
src/components/SourceFilterSheet.vue  # interface=alert for Type/Min rating; Settings-like sections
```

**Structure Decision**: Existing web-app layout; all changes in the three Discover client files that
own the search bar, chips, and filter sheet.

## Complexity Tracking

No constitution violations — section intentionally empty.

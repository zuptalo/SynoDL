# Implementation Plan: Order series download options by season then size

**Branch**: `fix/2004-series-download-options` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only. Extract the size parsing + a `bySeasonThenSize` comparator into a pure, unit-tested
module (`src/services/quality-sort.ts`) and use it in `SourceTitleModal.vue` for the visible options
list AND the default pre-selection, so series packs read season-ascending then largest-first while
movies stay largest-first.

## Technical Context
Vue 3 + Ionic (client only); no deps; no storage; vitest unit for the pure sort; PWA on k3s.

## Constitution Check
- I. Spec-Driven ✅ (spec 2004). III. Credential/state ✅ presentation only. TDD ✅ new pure module
  `quality-sort.ts` is unit-tested and added to the coverage gate (ratchet). V. Release-note ✅ `fix`.
No violations.

## Project Structure
```
src/services/quality-sort.ts        # new: sizeMB, seasonNum, bySeasonThenSize (pure)
src/services/quality-sort.test.ts   # new: unit tests (season-then-size, movies, parsing)
src/components/SourceTitleModal.vue  # use bySeasonThenSize for list + default; drop inline sizeMB
vitest.config.ts                     # add quality-sort.ts to the coverage include (ratchet)
```

## Complexity Tracking
None.

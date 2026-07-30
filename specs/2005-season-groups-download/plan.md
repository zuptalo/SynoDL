# Implementation Plan: Obvious season dividers in the download options list

**Branch**: `fix/2005-season-groups-download` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only, building on spec 2004's ordering. Add a pure `markSeasonBreaks()` to the existing
`quality-sort` module that tags each already-sorted option with `seasonBreak` (true when it opens a
new season, never for the first row or for season-less movie options), and render a stronger
accent-coloured top border + spacing on those rows in `SourceTitleModal.vue`.

## Technical Context
Vue 3 + Ionic (client only); no deps; no storage; vitest unit for the pure helper; PWA on k3s.

## Constitution Check
- I. Spec-Driven ✅ (spec 2005). III. Credential/state ✅ presentation only. TDD ✅ `markSeasonBreaks`
  unit-tested in the already coverage-gated `quality-sort` module. V. Release-note ✅ `fix`.
No violations.

## Project Structure
```
src/services/quality-sort.ts        # + markSeasonBreaks / GroupedOption
src/services/quality-sort.test.ts   # + break-flagging tests
src/components/SourceTitleModal.vue  # tag rows + .season-break styling
```

## Complexity Tracking
None.

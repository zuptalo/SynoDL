# Implementation Plan: Statistics filter segments sit in cards

**Branch**: `fix/2003-statistics-filter-segments` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only, one file (`src/components/StatisticsView.vue`). Mirror the Settings pattern: wrap the
source `ion-segment` (grouped with the admin user picker) and the history-range `ion-segment` in
`ion-item lines="none"` inside `ion-list inset` cards; segment fills the item (`.seg { width:100% }`),
replacing the old `ion-segment { margin }` float. No behaviour, server, or data change.

## Technical Context
Vue 3 + Ionic (client only); no deps; no storage; verified visually + `npm run build`; PWA on k3s.

## Constitution Check
- I. Spec-Driven ✅ (spec 2003). III. Credential/state ✅ presentation only, no state/API. TDD — n/a
  (pure CSS/markup; no logic; existing stats tests unaffected). V. Release-note ✅ user-facing `fix`.
No violations.

## Project Structure
`src/components/StatisticsView.vue` — segments wrapped in card items; `.seg` full-width.

## Complexity Tracking
None.

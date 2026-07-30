# Implementation Plan: Download statistics — readable history + totals

**Branch**: `feat/1017-download-statistics-readable` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/1017-download-statistics-readable/spec.md`

## Summary

Client-only. No server/API/data-model change.

- **DownloadsChart.vue**: replace the `preserveAspectRatio="none"` SVG (which stretches a single
  bucket into a full-box rectangle) with a responsive CSS/flex bar chart — bounded-width bars with
  rounded tops, a 2px gap, a baseline + faint max reference, direct value labels when few (tooltip
  otherwise), thinned x-labels + horizontal scroll when many, single accent-green series, empty state.
- **stats-buckets.ts**: make `bucketize('all')` adaptive — pick year/month/day granularity from the
  data span so "All" is a real overview instead of one "All time" bar. Update the pure-module tests.
- **StatisticsView.vue**: show a prominent total (count + total size) for the current user/source
  selection near the History section, reusing the already-computed `overall` aggregate.

## Technical Context

**Language/Version**: TypeScript / Vue 3 + Ionic (client only)
**Primary Dependencies**: none new — bespoke CSS chart (project keeps deps minimal)
**Storage**: none
**Testing**: vitest unit for `stats-buckets` (coverage-gated module — keep ≥ floors); the chart is
markup/CSS verified on the deployed build
**Target Platform**: PWA; k3s via Keel
**Project Type**: web application (client change only)
**Constraints**: theme-aware light/dark; value labels/tooltip so a single-hue chart isn't colour-alone
**Scale/Scope**: 3 client files + the stats-buckets test

## Constitution Check

- **I. Spec-Driven** — ✅ spec `1017`.
- **III. Custodial State & Credential Safety** — ✅ presentation only; no state, credentials, or API
  surface touched. Checklist gate not triggered.
- **TDD** — ✅ the `bucketize('all')` change is covered by the coverage-gated unit tests (updated
  first); the chart is CSS verified visually.
- **V. Release-note subject** — ✅ user-facing `fix(stats)` plain-language subject.

No violations → no Complexity Tracking.

## Project Structure

```text
src/components/DownloadsChart.vue      # rewrite: responsive CSS bar chart
src/services/stats-buckets.ts          # adaptive 'all' bucketing
src/services/stats-buckets.test.ts     # updated 'all' expectations + grain-selection cases
src/components/StatisticsView.vue      # total (count + size) for the selected user/source
```

**Structure Decision**: Existing layout; changes are confined to the statistics view, its chart, and
the pure bucketing helper.

## Complexity Tracking

No constitution violations — section intentionally empty.

# Implementation Plan: Release-year sort is fast again, and Discover opens on Most popular

**Branch**: `fix/2007-release-year-sort` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Server-only, in the 30nama driver. Remove spec 2006's implicit `min_year`/`max_year` injection from
`buildParams` (back to a plain filters-only body), and change `orderbyField`'s fallback from `year`
to `favorite` so an empty/unknown sort matches the client's own `DEFAULT_SORT`. The client already
defaults to `favorite`, so no client change is needed.

## Technical Context
Go 1.26 stdlib; no deps, no state, no allowlist change. Covered by the package's existing table
tests against the fake provider (no network).

## Constitution Check
- I. Spec-Driven ✅ (spec 2007, supersedes 2006). II. TDD ✅ the regression test asserting no
  implicit bounds is written first, and the previous spec's bounds test is replaced by it — a bug
  fix begins with a failing test. III. Custodial state ✅ request-shaping only. V. Release-note ✅
  `fix(discover):` in user language.
No violations.

## Project Structure
```
server/internal/source/providers/thirtynama.go       # drop the year bounds; default sort → favorite
server/internal/source/providers/thirtynama_test.go  # bounds-are-never-implicit + default-sort tests
```

## Complexity Tracking
None. This removes code rather than adding it.

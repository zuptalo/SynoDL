# Implementation Plan: Release-year sort no longer leads with year-less titles

**Branch**: `fix/2006-release-year-sort` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Server-only, inside the 30nama driver. `buildParams` gains the sort field and, when the resolved
`orderby` is `year`, fills in `min_year` / `max_year` for any bound the user hasn't set (1890 / 2100).
That excludes the source's year-less and garbage-year rows, which are the only reason the release-year
sort looks unsorted. No client change: the app already sends sort + direction and renders whatever
comes back.

## Technical Context
Go 1.26 stdlib (`internal/source/providers/thirtynama.go`); no deps, no state, no new API in the
allowlist — the same `advanced_search` call with two more values in the existing `parameters` body.
Tested with the package's existing table tests against the fake DSM/provider (no network).

## Constitution Check
- I. Spec-Driven ✅ (spec 2006). II. TDD ✅ `buildParams` / `Search` tests written first.
- III. Stateless, credential-free proxy ✅ request-shaping only: nothing persisted, no new host, no
  credential handling touched. V. Release-note subject ✅ `fix(discover): …` in user language.
No violations.

## Project Structure
```
server/internal/source/providers/thirtynama.go       # buildParams(f, sort) + year bounds
server/internal/source/providers/thirtynama_test.go  # bounds applied / user bounds win / other sorts untouched
```

## Complexity Tracking
None.

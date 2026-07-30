# Implementation Plan: Discover keeps loading ahead of a fast scroller

**Branch**: `feat/1018-discover-infinite-scroll` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only, two edits. `loadMore()` in `useSourceCatalog.ts` pulls `PAGES_PER_LOAD = 2` pages per
scroll trigger instead of one, appending each as it arrives and re-checking the existing bail-out
guards between them. `ion-infinite-scroll`'s threshold in `BrowserPage.vue` goes from `60%` to
`100%` so the trigger fires a full viewport before the end.

No server change, no new endpoint, no change to how many requests a given amount of scrolling
costs — only when they happen.

## Technical Context
Vue 3 + Ionic composable state; no deps; no storage. Vitest over the existing
`useSourceCatalog.test.ts` with the api module mocked. PWA on k3s.

## Constitution Check
- I. Spec-Driven ✅ (spec 1018). II. TDD ✅ pagination tests written before the change; e2e is not
  possible for a stateful-only feature (documented in the spec's Assumptions, consistent with the
  established stateless-harness workaround). III. Custodial state ✅ no stored data, no credentials,
  no NAS or provider allowlist change. V. Release-note ✅ `fix(discover):` in plain language.
No violations.

## Project Structure
```
src/composables/useSourceCatalog.ts       # PAGES_PER_LOAD loop in loadMore()
src/composables/useSourceCatalog.test.ts  # + pagination tests (api mocked)
src/views/tabs/BrowserPage.vue            # infinite-scroll threshold 60% → 100%
```

## Complexity Tracking
None.

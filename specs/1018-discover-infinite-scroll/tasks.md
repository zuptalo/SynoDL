# Tasks: Discover keeps loading ahead of a fast scroller

**Feature**: `specs/1018-discover-infinite-scroll` | **Branch**: `feat/1018-discover-infinite-scroll`

- [x] T001 [US1] Extend `src/composables/useSourceCatalog.test.ts` first (TDD), with the api module
  mocked: one trigger requests two consecutive pages; it stops at the last page instead of
  requesting past the end; it stops after the first page when the source errors mid-trigger.
- [x] T002 [US1] In `src/composables/useSourceCatalog.ts`, give `loadMore()` a named
  `PAGES_PER_LOAD` constant and loop, re-checking the hasMore/needs-refresh/unavailable/error
  guards between pages, with a comment recording why (fast scrollers meeting the spinner).
- [x] T003 [US1] In `src/views/tabs/BrowserPage.vue`, raise the `ion-infinite-scroll` threshold from
  `60%` to `100%` and update the neighbouring comment.
- [x] T004 Gate: `npm run build` + `npm run test:unit:coverage`; `make roadmap`. Manual on device:
  fast-scroll Discover without meeting the spinner; filter change still resets to page 1; an expired
  source session stops the load-ahead cleanly.

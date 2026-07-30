# Tasks: The IMDb rating opens the title on IMDb

**Feature**: `specs/1019-imdb-rating-links` | **Branch**: `feat/1019-imdb-rating-links`

- [x] T001 [US1] Write `src/services/imdb-link.test.ts` first (TDD): a bare `tt` id becomes the
  canonical URL; a full IMDb URL is normalised to the same; empty, whitespace, a non-`tt` value and
  anything with unexpected characters return `''`.
- [x] T002 [US1] Add `src/services/imdb-link.ts` with `imdbUrl()` satisfying those tests, and
  register it in `vitest.config.ts`'s coverage `include` list (Principle II ratchet).
- [x] T003 [US1] In `src/components/SourceTitleModal.vue`, render the IMDb rating as an anchor
  (`target="_blank"`, `rel="noopener noreferrer"`) when the URL is non-empty — including the
  score-less case, which shows just "IMDb" — and keep today's span when it isn't. Style it so it
  reads as a link without breaking the meta line.
- [x] T004 Gate: `npm run build` + `npm run test:unit:coverage`; `make roadmap`. Manual on device:
  tapping the rating opens the right IMDb page in the browser and the modal is still open on return.

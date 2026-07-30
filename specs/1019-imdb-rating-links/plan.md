# Implementation Plan: The IMDb rating opens the title on IMDb

**Branch**: `feat/1019-imdb-rating-links` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only. A new pure module `src/services/imdb-link.ts` turns an IMDb id into a canonical
`https://www.imdb.com/title/<id>/` URL, returning `''` for anything that isn't `tt` + digits (it
also tolerates a full URL, since the provider's detail endpoint returns one). `SourceTitleModal.vue`
renders the rating as an `<a target="_blank" rel="noopener noreferrer">` when that URL is non-empty,
and as today's `<span>` otherwise.

`rel="noopener noreferrer"` satisfies FR-005; `target="_blank"` is what an installed PWA hands to
the system browser (no Capacitor in this project, so there is no stronger mechanism available).

## Technical Context
Vue 3 + Ionic `<script setup>`; no deps, no server change, no storage. Vitest over the new pure
module, which joins the coverage allowlist per Principle II's ratchet.

## Constitution Check
- I. Spec-Driven ✅ (spec 1019). II. TDD ✅ `imdbUrl` unit-tested before use, and the new module is
  added to `vitest.config.ts`'s gated `include` list. III. Custodial state ✅ no stored data, no
  credentials, no server or allowlist change — the link is rendered client-side and the user's
  browser fetches IMDb directly. VI. Ionic-first ✅ a plain anchor inside the existing meta line;
  no new component. V. Release-note ✅ `feat(discover):` in user language.
No violations.

## Project Structure
```
src/services/imdb-link.ts        # imdbUrl(id) → canonical URL or ''
src/services/imdb-link.test.ts   # id shapes, full URL, junk, empty
src/components/SourceTitleModal.vue  # anchor when linkable, span otherwise
vitest.config.ts                 # add imdb-link.ts to the coverage allowlist
```

## Complexity Tracking
None.

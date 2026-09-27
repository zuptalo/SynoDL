# Feature Specification: ZarFilm browses through the site's advanced search

**Feature Branch**: `feat/1046-zarfilm-browses-through`

**Created**: 2026-09-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "let's now run an investigation on the search capability in zhomis.info (zarfilm), it is actually way more complete than you initially though!" — with a captured advanced-search request and result page.

## Overview

Spec 2037 restored ZarFilm's catalog after the site's redesign, but the
redesign also removed the archive filter panel the driver read its abilities
from, so Discover could narrow ZarFilm by nothing but Movie/Series.

The site's own advanced-search dialog expresses far more. Verified live on
2026-09-27 (signed in, every option tried):

- one GET on the site root, `/[page/N/]?advsearch=on&…`, answered with the same
  cards the archive uses, 21 per page, with normal pagination
- `mobile_type` all/post/series · `mobile_advsgenre` (28 Persian genre names)
  · `search_order` 1 newest / 2 views / 4 user rating / 5 votes / 6 IMDb ·
  `languageSearch` (133 English names) · `advscountry` (~117 Persian names) ·
  `advsqulity` (~68 exact release labels) · toggles dubbed / subtitle / 3D /
  online / 4K · `imdbID` · an IMDb range · a year range
- all of it composes (Korean + comedy + 2015–2026 + top IMDb → Parasite first)
- **a range works only with both ends** — one end alone is silently dropped
- **the 4K toggle always returns nothing** (a site bug)
- **a text search (`s=`) ignores every advanced field and the ordering**
- the dialog itself is fetched by the site's scripts from
  `POST /wp-admin/admin-ajax.php` with `action=get_advanced_search` (no nonce),
  answered as `{"stat":"ok","html":"…"}`

## Requirements

- **FR-001**: A browse (no text) MUST be the advanced search: every form field
  sent, with its neutral value where the user chose nothing, paged as
  `/page/N/`.
- **FR-002**: Type, genre, ordering (newest / most popular = views / IMDb),
  language, country, quality, 3D, IMDb band and year range MUST be sent in the
  site's own vocabulary. A score band and a year range MUST be sent with both
  ends (a band "8+" as 8–10, "under 5" as 0–5; a missing year end as 1900 or the
  current year). The 4K toggle MUST never be set.
- **FR-003**: A text search MUST send only `s`, and MUST NOT claim an ordering;
  the type filter is applied to what comes back.
- **FR-004**: `Parameters()` MUST read the option lists from the dialog itself
  (the same AJAX call), so a genre or language the site adds appears without a
  release. Values stay as served; genres carry the English slug from the
  archive's genre routes, languages an ISO code from a built-in table of the
  site's English names. Score bands are declared with the shared `score-N` /
  `score-under-5` slugs; sorts in the canonical vocabulary — no release-year
  ordering, which the site cannot do.
- **FR-005**: A dialog that cannot be fetched or parsed MUST degrade to the
  known facets (types, year range), never fail the browse (0007 FR-011).
- **FR-006**: Dev/e2e: the fake site serves the dialog and honours the
  advanced parameters it reads (type, genre, IMDb lower bound, ordering 6).

## Out of scope

- Dubbed / subtitle toggles as SynoDL filters (ZarFilm-only; no shared field).
- Ascending order: the site has none; the toggle leaves ZarFilm's order as is.
- Cross-source joining of what this declares — spec 1047.

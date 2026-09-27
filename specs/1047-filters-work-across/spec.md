# Feature Specification: Filters that work across every source

**Feature Branch**: `feat/1047-filters-work-across`

**Created**: 2026-09-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "now look into a combine filtering functionality which maps both 30nama and zarfilm so when we are filtering we get the relevant results from all active sources"

## Overview

Spec 0007 decided that "All sources" offers only the filters every source can
honour (union with per-source application was rejected), and spec 1024 built
the machinery: facets joined by English slug or label, one chosen value
translated into each source's own word before the fan-out. With two real
sources it produced almost nothing, for reasons the machinery hid:

1. **Types never joined.** 30nama declares its types as its own numeric codes
   with Persian labels; ZarFilm as `movie`/`series`. No slug on 30nama's side,
   so the Persian label was the key and Movie/Series dropped out of combined
   mode entirely. Type was also exempt from translation on the assumption its
   values were canonical — which 30nama's declared options are not.
2. **Languages never joined.** 30nama's are ISO codes with Persian labels, no
   slug; ZarFilm's (spec 1046) are English names. Nothing shared.
3. **Countries** are Persian labels on both — but typed on different keyboards
   (Arabic vs Persian yeh/kaf, zero-width non-joiners), so several failed to
   fold together.
4. The option shown for a shared choice was the first source's, whose value
   may be a Persian word the client cannot label; the slug that could be
   labelled was discarded.
5. **An IMDb ordering across sources was alternated**, source by source: a 9.8
   beside a 7.1 beside a 9.5.

## Requirements

- **FR-001**: 30nama's type options MUST carry the canonical slugs
  (`movie`/`series`/`anime`); its language options MUST carry their ISO code as
  slug; its "below 5" band MUST carry `score-under-5`, matching ZarFilm.
- **FR-002**: Type MUST be translated per source like every other facet; a
  value shared verbatim round-trips.
- **FR-003**: Facet labels MUST fold Arabic yeh/kaf to Persian and drop the
  zero-width non-joiner before comparison.
- **FR-004**: The option shown for a shared choice MUST carry a slug when any
  source has one; the client MUST label languages and types from the slug
  first, then the value. Countries join by their Persian label (neither source
  slugs them) and are shown from the first source's option — an ISO code when
  30nama leads, which the client names in English.
- **FR-005**: With the IMDb ordering, combined results MUST be merged by rating
  (descending, or ascending when asked), unrated titles last; other orderings
  keep the round-robin of spec 0007.
- **FR-006**: A test MUST assert that the intersection of the two real
  drivers' declared facets (30nama's captured parameters, ZarFilm's captured
  dialog) is non-empty for types, genres, languages, countries, scores and
  sorts — the check that would have caught the empty sheet.

## What stays 30nama-only or ZarFilm-only

Quality (coarse vs exact release labels), channel, encoder, age rating, x265,
cast/director/creator, the ascending order, and ZarFilm's dubbed/subtitle
toggles. Per 0007 FR-014 they are offered when that source is browsed alone.

## Out of scope

- Union mode (every filter from every source, each applied where it works):
  rejected in 0007; unchanged here.
- De-duplicating a title carried by both sources (0007 FR-012a: kept, labelled).

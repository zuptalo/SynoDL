# Feature Specification: ZarFilm's catalog shows titles again after the site's redesign

**Feature Branch**: `fix/2037-zarfilm-new-card-layout`

**Created**: 2026-09-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "I have now configured https://zhomis.info, provided the Cookie header and User-Agent header and it was happy and saved the source, but when trying to see the items from ZarFilm in the Discover it says No Results! … have a look a the logs and fix the issue please"

## Overview

With the source configured at `https://zhomis.info` (spec 1045) it verified and
saved, and every browse returned **200, 1338 pages, 0 items**.

The site has a new theme. Checked live on 2026-09-27 in a signed-in browser:
result cards are now `.zf-card-item` (24 per page); the `.inner_item_body_widget`
/ `a.bgbackitem` markup the listing parser looked for appears nowhere. Links are
on `zhomis.info` (so not an address problem), and the page is logged in (so not a
session problem) — only the card parser was blind. Title pages (movie download
rows, series seasons, cast, genres, IMDb id) are unchanged and still parse. The
old archive filter panel is gone site-wide; the driver's existing no-panel path
covers that.

## Requirements

- **FR-001**: The listing parser MUST read the current card (`a.zf-card-link`,
  `h3.zf-card-title`, `.zf-card-year`, `.zf-card-rating b`, `img.zf-card-image`,
  `.zf-card-genre-chips span`), identifying a series by its `series/` path, and
  taking a movie's year from its slug when the card has none.
- **FR-002**: The older card MUST still be read, since a theme can roll out
  unevenly.

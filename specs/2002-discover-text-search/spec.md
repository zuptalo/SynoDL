# Feature Specification: Fix Discover text search filtering

**Feature Branch**: `fix/2002-discover-text-search`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Discover text search drops the type filter (returns nothing) and silently ignores sort and facet filters."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Searching with a Type filter returns matching titles (Priority: P1)

A user opens Discover, sets the type filter to **Movies** (or Series, or Anime), and
types a search term. Today they get an **empty results screen** — the search silently
returns nothing. They expect to see the matching titles of that type.

**Why this priority**: This is a data-loss-shaped bug: a common, reasonable combination
(pick a type, then search) produces zero results with no explanation, making the whole
search feature look broken. It is the reason this spec exists.

**Independent Test**: With the type filter set to Movies, search a well-known term (e.g.
"batman") and confirm matching movie titles appear; switch the type to Series and confirm
the results change to matching series.

**Acceptance Scenarios**:

1. **Given** the type filter is set to Movies, **When** the user searches a term with
   known movie matches, **Then** matching movie titles are shown (not an empty screen).
2. **Given** the type filter is set to Series, **When** the user searches the same term,
   **Then** only matching series are shown.
3. **Given** no type filter is set, **When** the user searches, **Then** matches of all
   types are shown (unchanged from today).

---

### User Story 2 - Sort and advanced filters don't pretend to work during search (Priority: P2)

A user runs a text search and sees the sort control and the advanced facet filters (genre,
release year, language, country, rating, quality) still fully interactive. Changing them
appears to do nothing, because the source cannot sort or facet-filter text-search results.
The user is left confused about whether the app is broken.

**Why this priority**: The controls actively mislead. Making them honest removes a whole
class of "sorting/filtering doesn't work" confusion, but it is a smaller trust/clarity fix
than the empty-results bug, so it rides second.

**Independent Test**: Start a text search; confirm the sort control and the non-type facet
filters are visibly disabled with a short hint, and that clearing the query re-enables them.

**Acceptance Scenarios**:

1. **Given** a text query is active, **When** the user looks at the sort control and the
   genre/year/language/country/rating/quality filters, **Then** they are visibly disabled
   with a brief hint that they apply to browsing only.
2. **Given** a text query is active, **When** the user looks at the type filter, **Then** it
   remains enabled and usable (per User Story 1).
3. **Given** the user clears the query, **When** the search box is empty, **Then** the sort
   control and all facet filters become active again and browse behaves exactly as before.

---

### Edge Cases

- A search whose matches are all of a different type than the active type filter yields an
  honest empty state ("no matching Movies"), not a raw error.
- A page of raw results that, after type re-filtering, is thinner than a screenful still
  fills the grid (pagination continues until there is enough, as browse already does).
- Switching the type filter while a query is active re-runs the search against the new type.
- Toggling the query on and off must not lose the user's previously chosen sort/filters —
  they are restored for browsing when the query is cleared.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A text search with an active type filter MUST return the matching titles of
  that type, never an empty result caused by the type filter itself.
- **FR-002**: The type filter MUST continue to narrow text-search results to the selected
  type (Movies, Series, or Anime).
- **FR-003**: A text search with no type filter MUST return matches across all types
  (unchanged behaviour).
- **FR-004**: While a text query is active, the sort control and the non-type facet filters
  (genre, release year, language, country, rating, quality, and any advanced facets) MUST be
  presented as disabled, with a short hint that they apply to browsing.
- **FR-005**: Clearing the query MUST re-enable the sort control and all facet filters and
  restore normal browse behaviour, without losing the user's saved sort/filter choices.
- **FR-006**: Browsing (no query) MUST be unchanged: sort (most popular / IMDb / year /
  recently added), sort direction, and every facet filter continue to work as they do today.
- **FR-007**: The credential-free, stateless-proxy boundary MUST be preserved — no new
  stored state, no credentials in logs, and no source API beyond the existing allowlist.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Searching any common term with a Type filter active returns a non-empty,
  correctly-typed result set in 100% of cases where such titles exist (today: 0%).
- **SC-002**: 0 misleading interactions — during a text search, no disabled-in-effect
  control appears interactive; users cannot change a setting that has no effect.
- **SC-003**: Browse behaviour is equivalent before and after the change (no regression in
  sort or filtering when the query is empty).

## Assumptions

- The source (30nama) genuinely cannot sort or facet-filter text-search results; this was
  verified against the live API. The fix therefore makes the UI honest rather than trying to
  force server-side behaviour the source does not support.
- The type filter can be honoured for text search by narrowing the returned titles to the
  selected type, using the type already present on each result.
- "Disable with a hint" is preferred over hiding the controls, so the user understands the
  controls still exist and will work again when they clear the query (decided with the user).

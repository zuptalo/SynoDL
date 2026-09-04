# Feature Specification: Filter and sort every source the same way

**Feature Branch**: `feat/1024-filter-and-sort`

**Created**: 2026-09-04

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "have another look at the zarfilm ui and see what sort of searching and categories are available, if they have something I would like us to map them as much as possible to what we have in 30nama, then when all sources are selected we can show the options which they both have in common for filtering and sorting, when one of them is selected then we show the full options available to that specific source, zarfilm might have the farsi names in the ui for genres, but I'm sure you can map them as best as possible"

## Context

Discover's filter sheet is built from whatever the selected source says it can
filter on. 30nama publishes a full facet set, so filtering and sorting work.
ZarFilm publishes only "movie / series", so the sheet is nearly empty whenever
ZarFilm is selected, and empty of everything but type whenever both sources are
combined — because the combined view offers only what every source can do.

ZarFilm can in fact do more. Its archive pages carry a filter panel with three
groups: a sort order, an IMDb-score band, and a genre. None of it is offered in
SynoDL today, and the sort the driver does attempt is written with the wrong
parameter name, so it has never had any effect.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Filter and sort ZarFilm (Priority: P1)

A user browsing with ZarFilm selected opens the filter sheet and finds it almost
empty: no genres, no score, no sort. The catalog is tens of thousands of titles
deep and the only way through it is to keep scrolling.

After this change the sheet offers ZarFilm's own genres, its IMDb-score bands and
its sort orders, and choosing one actually changes the results.

**Why this priority**: Without it there is nothing to intersect for the combined
view either — every other story in this spec depends on ZarFilm declaring what it
can do.

**Independent Test**: Select ZarFilm alone, pick a genre, a score band and a sort
order, and confirm the grid changes accordingly and paginates correctly.

**Acceptance Scenarios**:

1. **Given** ZarFilm is the selected source, **When** the user opens the filter
   sheet, **Then** it offers genres, score bands, types and sort orders.
2. **Given** a genre is chosen, **When** the results load, **Then** every result
   belongs to that genre, for a movie browse and for a series browse alike.
3. **Given** a sort order is chosen, **When** the results load, **Then** the order
   changes — sorting is not silently ignored.
4. **Given** a filter and a sort are both set, **When** the user scrolls to the
   next page, **Then** both still apply.
5. **Given** a genre is chosen while browsing series, **When** the results load,
   **Then** the genre is applied using the vocabulary the series archive uses,
   not the movie one.

### User Story 2 - Combined browsing offers what every source can do (Priority: P2)

With both sources combined, the sheet must offer only what all of them support —
otherwise a filter would apply to half the results and quietly not to the rest.
The mechanism for that already exists; today it collapses to almost nothing
because ZarFilm declares almost nothing.

**Why this priority**: This is the user's stated goal, but it is a consequence of
US1 rather than separate work.

**Independent Test**: With both sources enabled and none selected, open the sheet:
genre, type and score are offered; 30nama-only facets are not.

**Acceptance Scenarios**:

1. **Given** both sources are combined, **When** the sheet opens, **Then** it
   offers the facets both sources support and omits those only one supports.
2. **Given** a genre is chosen in combined mode, **When** results load, **Then**
   both sources return that genre — each having interpreted it in its own
   vocabulary.
3. **Given** a sort order is chosen in combined mode, **When** results load,
   **Then** only sort orders both sources support are offered.

### User Story 3 - A single source offers everything it can do (Priority: P2)

Selecting one source must not cost the user that source's own capabilities.

**Independent Test**: Select 30nama alone; the sheet still offers quality,
country, language, channel, encoder, age and the year range. Select ZarFilm
alone; the sheet offers ZarFilm's three groups and no facet ZarFilm cannot honour.

**Acceptance Scenarios**:

1. **Given** 30nama is selected alone, **When** the sheet opens, **Then** its full
   facet set is offered, unchanged from today.
2. **Given** the user switches from combined to a single source, **When** the sheet
   reopens, **Then** the newly available facets appear.
3. **Given** a filter was set that the newly selected source cannot honour, **When**
   the source changes, **Then** that filter is dropped rather than silently ignored.

### Edge Cases

- A genre one source has and the other does not (ZarFilm has film-noir and
  game-show; 30nama's list differs) must simply not appear in the combined sheet.
- A source that fails to report its capabilities must not empty the sheet for the
  others, and must not fail the search.
- Text search and browse are different operations on both sources: where a source
  cannot apply facets to a text search, the sheet must not offer facets that would
  do nothing.
- A genre label is Persian on one route and an English slug on another for the
  same genre; both must resolve to the same filter.
- A stored filter naming a value that no longer exists must not wedge Discover.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The ZarFilm source MUST declare the facets it can honour: genres,
  types and IMDb-score bands.
- **FR-002**: The ZarFilm source MUST declare the sort orders it can honour, and
  MUST apply a chosen sort using the parameter the site actually reads. Sorting
  MUST have a visible effect.
- **FR-003**: A genre chosen for ZarFilm MUST be applied for a movie browse and
  for a series browse, each using the vocabulary that route accepts.
- **FR-004**: A genre, score and sort MUST compose — choosing all three MUST apply
  all three, and MUST survive pagination.
- **FR-005**: Facet values MUST be expressed to the client in one vocabulary
  shared across sources, so a single chosen value is meaningful to every selected
  source.
- **FR-006**: The server MUST translate a chosen facet value into each source's
  own vocabulary when searching, so drivers keep their native filter semantics.
- **FR-007**: A value a given source has no equivalent for MUST NOT be sent to
  that source as if it were valid.
- **FR-008**: In combined mode the sheet MUST offer only facets and sort orders
  every selected source supports.
- **FR-009**: With a single source selected the sheet MUST offer that source's
  full set.
- **FR-010**: Genre labels MUST be shown in a form the user can read, and MUST be
  stable for the same genre regardless of which source contributed it.
- **FR-011**: A source that cannot report its capabilities MUST degrade to
  offering nothing for itself without emptying or failing the sheet for others.
- **FR-012**: Where a source cannot apply facets to a text search, choosing one
  MUST NOT silently return unfiltered results presented as filtered.
- **FR-013**: 30nama's existing filtering, sorting and facet set MUST be unchanged
  from the user's point of view.

### Key Entities

- **Facet** — one filterable dimension (genre, type, score, quality …) and the
  values a source offers for it.
- **Facet value vocabulary** — the shared identity of a value across sources
  (e.g. "the comedy genre"), distinct from the code any one source uses for it.
- **Sort order** — how a browse is ordered; a per-source capability like a facet.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With ZarFilm selected, a user can narrow the catalog by genre and by
  score, and change the ordering, without leaving SynoDL.
- **SC-002**: A genre chosen in combined mode returns that genre from both
  sources.
- **SC-003**: Selecting a single source never offers fewer facets than combined
  mode does.
- **SC-004**: No facet is offered that has no effect when applied.
- **SC-005**: 30nama browsing behaves exactly as it does today.
- **SC-006**: One source failing to report capabilities costs the user only that
  source's facets.

## Assumptions

- ZarFilm publishes no quality, country, language, channel, encoder, age or
  release-year facet, so those stay 30nama-only and appear only when 30nama is
  selected alone.
- ZarFilm's genre vocabulary can be aligned with 30nama's by English slug: the
  site's own movie-genre routes pair each Persian label with an English slug, so
  the mapping is read from the source rather than hand-written. The few genres
  with no such route need an explicit slug.
- 30nama's own facet values remain the vocabulary it is sent; the shared
  vocabulary is what the client speaks, not what any driver speaks.

## Credential-Safety Impact

- No change to the DSM API allowlist and no NAS call added or widened.
- No new download source, host or endpoint: every request goes to hosts the
  sources' existing allowlists already permit, using the caller's own source
  session.
- Capability data is per-source configuration, not user data. It is cached in
  memory only; nothing about it is persisted, and it holds no session material.
- No credential, session cookie or signed URL enters a log through this change;
  facet values and genre labels are catalog vocabulary, not user content.

# Feature Specification: ZarFilm titles carry an IMDb link and a synopsis

**Feature Branch**: `feat/1023-zarfilm-titles-carry`

**Created**: 2026-09-04

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "we do show the item details and ratings and link to the IMDB for things we pull from 30nama, can we make sure the same is also implemented for zarfilm contents? … it is ok to show the farsi description for the titles if there is no english version"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Read what a ZarFilm title is about (Priority: P1)

A user browsing Discover against a ZarFilm source opens a movie or a series.
Today the sheet shows the poster, the rating and the download options, and
nothing else — no synopsis at all — while the same sheet opened against a
30nama title carries a paragraph describing the title. The user has to leave the
app to find out what the film is before deciding to spend a download on it.

After this change the sheet shows the title's synopsis. ZarFilm publishes it in
Persian only, and a Persian synopsis is shown as published rather than withheld:
the audience for a Persian-language source reads it, and something in the wrong
language still beats nothing.

**Why this priority**: This is the larger half of the gap the user reported, and
it is the part that changes a decision — whether to download the title at all.

**Independent Test**: Open a ZarFilm movie and a ZarFilm series in Discover; each
sheet shows a synopsis paragraph, laid out right-to-left, and the sheet still
works normally for a title that has no synopsis.

**Acceptance Scenarios**:

1. **Given** a ZarFilm movie whose page publishes a synopsis, **When** the user
   opens it from Discover, **Then** the sheet shows that synopsis under the header.
2. **Given** a ZarFilm series whose page publishes a synopsis, **When** the user
   opens it, **Then** the sheet shows it, in the same place and style as a 30nama
   title's synopsis.
3. **Given** a Persian synopsis, **When** it is shown, **Then** it reads
   right-to-left with its punctuation in the right place, without forcing the
   rest of the sheet into a right-to-left layout.
4. **Given** a ZarFilm title whose page publishes no synopsis, **When** the user
   opens it, **Then** the sheet renders exactly as it does today — no empty
   paragraph, no error.

### User Story 2 - Follow a ZarFilm title through to IMDb (Priority: P2)

The same user wants the rating in context: cast, reviews, what else the director
made. For a 30nama title the rating in the sheet header is a link out to IMDb
(spec 1019). For a ZarFilm title it is plain text, because nothing in the app
knows the title's IMDb id — even though the ZarFilm page itself links to IMDb.

After this change the rating on a ZarFilm sheet is the same link out.

**Why this priority**: Smaller than the synopsis and useful only once the user is
already interested, but it is the other half of the reported gap and it reuses
machinery that already exists on both sides.

**Independent Test**: Open a ZarFilm title whose page links to IMDb; the rating in
the header is tappable and opens that title's IMDb page in the device browser.

**Acceptance Scenarios**:

1. **Given** a ZarFilm title whose page carries an IMDb link, **When** the user
   opens the sheet, **Then** the rating is a link to that IMDb title.
2. **Given** a ZarFilm title with no discoverable IMDb id, **When** the user opens
   the sheet, **Then** the rating renders as plain text exactly as today.
3. **Given** a title opened from a download in Tasks rather than from Discover,
   **When** its sheet loads, **Then** it gains the same synopsis and link.

### Edge Cases

- A title page that publishes the synopsis twice (the site repeats it for its
  mobile layout) must produce one synopsis, not two concatenated copies.
- A synopsis that is only whitespace, or a placeholder dash, counts as absent.
- A very long synopsis must not push the download options off the sheet.
- A malformed or truncated title page must not fail the sheet: the download
  options are the point of the request, and missing metadata is not an error.
- A title whose page is reachable but whose downloads are paywalled behaves as it
  does today (the sheet reports the entitlement problem); this change adds no new
  outcome there.
- A synopsis containing HTML entities or markup must be shown as text, never as
  markup.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A title's detail response MUST be able to carry the title's synopsis
  and its IMDb id, for every source kind, not only for sources that already put
  them in catalog search results.
- **FR-002**: The ZarFilm source MUST read the synopsis from the title page it
  already fetches to build the download options. No additional request to the
  source is made for metadata.
- **FR-003**: The ZarFilm source MUST read the title's IMDb id from the IMDb link
  the title page carries.
- **FR-004**: Synopsis and IMDb id MUST be read for both movie and series pages.
- **FR-005**: Where the site publishes the synopsis more than once on a page, the
  result MUST be a single copy.
- **FR-006**: A synopsis that is empty, whitespace, or punctuation-only MUST be
  treated as absent.
- **FR-007**: The synopsis MUST be carried and shown as plain text; any markup or
  character entities in the source page MUST be resolved to text.
- **FR-008**: When the catalog entry the sheet was opened with already carries a
  synopsis or IMDb id (as a 30nama entry does), that value MUST win; detail values
  fill gaps only. A source that supplies richer metadata is never overwritten by a
  thinner one.
- **FR-009**: The sheet MUST show a Persian synopsis in right-to-left reading
  order, without changing the direction of the rest of the sheet.
- **FR-010**: Failure or absence of either value MUST NOT fail the title request:
  the sheet still lists download options.
- **FR-011**: Behaviour for 30nama titles MUST be unchanged.

### Key Entities

- **Title detail** — what the app asks a source for when a user opens a title:
  today the download options and what is already on the NAS; after this change
  also the title's synopsis and IMDb id.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Opening a ZarFilm title that publishes a synopsis shows it, for both
  movies and series.
- **SC-002**: Opening a ZarFilm title whose page links to IMDb gives a working way
  through to that IMDb page, in the same place a 30nama title offers one.
- **SC-003**: No ZarFilm title takes longer to open than it does today — the
  metadata comes from a page already being fetched, so the number of requests to
  the source is unchanged.
- **SC-004**: A ZarFilm title with neither value opens exactly as it does today.
- **SC-005**: 30nama titles show the same synopsis, rating and IMDb link as before.

## Assumptions

- ZarFilm publishes no English synopsis. The user has confirmed the Persian one is
  what should be shown ("it is ok to show the farsi description for the titles if
  there is no english version"), so no translation, and no English-only gate.
- Cast, director and country are published by ZarFilm but are out of scope: the
  app has no field for them and 30nama does not supply them, so adding them would
  make the two sources diverge rather than converge, which is the opposite of what
  was asked.
- The Discover grid is unchanged. The gap the user reported is in the title sheet;
  the grid card shows a poster, a rating and a year for both sources already.
- ZarFilm listing pages carry IMDb ids only inside poster filenames. That is too
  incidental to depend on, which is why the id is read from the detail page's real
  IMDb link instead.

## Credential-Safety Impact

- No change to the DSM API allowlist, and no NAS call is added or widened.
- No new request to any download source: both values are parsed from the title
  page the source driver already fetches, using the caller's own source session.
- Nothing new is persisted. Synopsis and IMDb id travel in the title response and
  are held only for the life of the open sheet, like the download options beside
  them.
- No new value reaches a log. Parsing failures are silent by design (FR-010), so
  there is no error path that could carry page content, a source session cookie,
  or a signed download URL into a log line.
- The new values ride on the existing title endpoint, which already resolves the
  title through the caller's own source access. A user therefore learns nothing
  about a title they could not already open.

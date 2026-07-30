# Feature Specification: The IMDb rating opens the title on IMDb

**Feature Branch**: `feat/1019-imdb-rating-links`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Make the IMDb rating in a title's detail view a link to that title's IMDb page, opening
in the device's normal browser rather than inside the app.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Check a title on IMDb before downloading (Priority: P1)

Looking at a title in Discover, the user wants to read more about it — cast, reviews, whether it's
the film they're thinking of — before committing to a download. The rating is already on screen and
is the natural thing to tap.

**Why this priority**: It's the one piece of outside context the decision usually needs, and today
it's a dead end: the user has to leave the app and search IMDb by hand.

**Independent Test**: Open a title's detail view and tap the IMDb rating; the title's IMDb page
opens in the browser, and the app is still where it was on return.

**Acceptance Scenarios**:

1. **Given** a title with an IMDb id, **When** the user taps the IMDb rating in the detail view,
   **Then** that title's IMDb page opens outside the app.
2. **Given** a title with an IMDb id but no rating yet, **When** the detail view renders, **Then**
   an IMDb link is still offered (there is a page to visit even without a score).
3. **Given** a title with no IMDb id, **When** the detail view renders, **Then** the rating shows as
   plain text exactly as it does today, with nothing to tap.
4. **Given** the user returns from IMDb, **When** the app comes to the foreground, **Then** the
   detail view is still open on the same title.

### Edge Cases

- An IMDb id in an unexpected shape (empty, a full URL, or anything not `tt` + digits) MUST NOT
  produce a broken or unsafe link — it falls back to plain text.
- The link must not hand the opened page any control over the app that opened it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A title's IMDb rating in the detail view MUST link to that title's page on IMDb when
  an IMDb id is known.
- **FR-002**: The link MUST open outside the app — in the device's browser — rather than replacing
  the app's own view.
- **FR-003**: A title with an IMDb id but no rating MUST still offer the link.
- **FR-004**: An absent or malformed IMDb id MUST fall back to today's plain-text rendering.
- **FR-005**: The opened page MUST NOT be able to reach back into the app that opened it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: From a title's detail view, reaching its IMDb page takes one tap instead of leaving
  the app and searching by hand.
- **SC-002**: Returning from IMDb leaves the app exactly where it was.

## Assumptions

- SynoDL is a plain PWA (no Capacitor), so "open externally" is `target="_blank"` — which an
  installed PWA hands to the system browser. There is no native bridge to do more than that.
- The Tasks detail view also shows an IMDb rating, but tasks store no IMDb id (only the catalog id,
  year, score and poster), so it is out of scope here — linking it would mean persisting another
  field at send time.

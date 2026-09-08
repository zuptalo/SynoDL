# Feature Specification: Show release year and readable genres on titles

**Feature Branch**: `feat/1032-show-release-year`

**Created**: 2026-09-07

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "let's also add the genre to the title cards ... we also can add and show the release year and genres inside the details view as well, maybe even language and the country ... we have a mixture of sources, zarfilm and 30nama, 30nama seems to be more in english and zarfilm provides some informations in farsi, let's make sure we cover them both"

## Overview

A title in Discover currently tells you three things: its name, its rating, and
that it is a Movie. Two of those are often not what you need. When a type filter
is already applied, every card repeats the word the filter just set — a whole
line of meta saying nothing. Meanwhile the two facts that actually separate one
title from another, **when it was made** and **what kind of film it is**, are
either missing or unreadable.

Both facts are already in the app. The year is parsed from the source and thrown
away; the genres arrive but in one source's own language, so a card can read
half in English and half in Farsi depending on which source it came from. This
feature spends what has already been fetched: a year where the source knows one,
and genres that read the same regardless of which source supplied them.

The cross-source part is the substance. One source publishes genres as English
slugs; the other publishes them as Farsi display names, and separately publishes
the mapping between the two for its own filter menu. Using that mapping for
titles as well as filters is what makes "Drama" mean Drama on every card, and
what lets the same genre from two sources read as one thing rather than two.

## User Scenarios & Testing

### User Story 1 - Tell titles apart at a glance (Priority: P1)

Someone browsing Discover sees, under each poster, the rating, the year it was
released, and what kind of film it is — in English, whichever source it came
from. They can tell a 1974 thriller from a 2024 one without opening either.

**Why this priority**: It is the whole point. A grid of posters where every
caption reads the same tells you nothing you could not see from the artwork.

**Independent Test**: Browse with both sources configured and confirm each card
shows a year (where the source knows one) and at least one English genre, with
no Farsi text on any card.

**Acceptance Scenarios**:

1. **Given** a title whose source publishes a release year, **When** the user
   browses Discover, **Then** the card shows that year alongside the rating.
2. **Given** a title from either source, **When** the user browses Discover,
   **Then** any genre shown is in English, and the same genre reads identically
   whichever source supplied the title.
3. **Given** a title whose source publishes no year, **When** the user browses
   Discover, **Then** the card shows no year rather than a placeholder or a
   guess.
4. **Given** a type filter is active, **When** the user browses Discover,
   **Then** cards do not repeat that type, because the filter already said it.
5. **Given** a card on a narrow phone screen, **When** it renders, **Then** its
   caption stays on its existing number of lines and does not truncate the title.

---

### User Story 2 - See the same facts when opening a title (Priority: P2)

Opening a title shows its year next to the type and rating, and its genres in
the same English vocabulary as the card that was tapped. Nothing changes
identity between the grid and the sheet.

**Why this priority**: A detail view that contradicts the card it came from is
worse than one that says less. This is small once Story 1 exists, but it is what
makes the two views feel like one app.

**Independent Test**: Open a title from the grid and confirm the year and genres
in the sheet match what the card showed.

**Acceptance Scenarios**:

1. **Given** a title with a known year, **When** the user opens its details,
   **Then** the year appears in the header beside the type and rating.
2. **Given** a title from the Farsi-publishing source, **When** the user opens
   its details, **Then** its genres read in English, matching the card.
3. **Given** a title reached without a catalog entry behind it, **When** the user
   opens its details, **Then** the header omits what it does not know rather
   than showing an empty row.

---

### Edge Cases

- **A source that publishes a wrong year.** One source is known to carry a body
  of titles with implausible release years. A year outside a sane range must not
  be shown at all — a missing year is a small gap, a wrong one is misinformation.
- **A genre with no known English equivalent.** A new or unmapped genre must
  still show something readable rather than vanishing or rendering as a raw slug
  with hyphens.
- **A title with many genres.** The caption must not grow or wrap; there is a
  fixed budget of what fits under a poster.
- **A year embedded in the title text.** Where a source publishes no separate
  year but puts one at the end of the title, that year should still be usable —
  and must not then appear twice, once in the title and once as the year.
- **Mixed sources in one grid.** Cards from both sources sit side by side and
  must be visually consistent; nothing should reveal which source a title came
  from except the existing source label.
- **A series with a year range.** The caption must handle a range as gracefully
  as a single year without breaking the line budget.

## Requirements

### Functional Requirements

- **FR-001**: A title card MUST show the title's release year when its source
  publishes one, and MUST show nothing in its place when it does not.
- **FR-002**: A title card MUST show at least one genre when the source
  publishes any.
- **FR-003**: Every genre shown to a user MUST be in English, regardless of
  which source supplied the title.
- **FR-004**: The same genre supplied by different sources MUST render as the
  same words, so it reads as one genre rather than two.
- **FR-005**: A genre for which no English equivalent is known MUST still render
  as readable words, never as a raw internal value.
- **FR-006**: System MUST NOT show a release year that falls outside a plausible
  range, because one source is known to publish implausible years for a body of
  its titles.
- **FR-007**: A card MUST NOT display the title's type while a filter for that
  type is active.
- **FR-008**: A card's caption MUST occupy no more vertical space than it does
  today, and MUST NOT cause the title to truncate.
- **FR-009**: The details view MUST show the title's release year alongside its
  type and rating when known.
- **FR-010**: The details view MUST show genres in the same English vocabulary
  as the card it was opened from.
- **FR-011**: Where a year appears both as its own field and at the end of the
  title text, it MUST be shown once.
- **FR-012**: Adding these facts MUST NOT change how many requests are made to a
  source, nor how long browsing takes to appear.

### Key Entities

- **Catalog title** — what the grid renders: its name, poster, rating, type,
  genres, and (new) its release year.
- **Genre** — a category as the user reads it. It has an English name, and each
  source has its own way of naming the same thing; the English name is what the
  user ever sees.

## Success Criteria

- **SC-001**: Browsing a mixed grid, a user can state each title's release
  decade and kind without opening any of them.
- **SC-002**: No Farsi text appears on any title card or in any details header,
  from either source.
- **SC-003**: A genre common to both sources reads identically on cards from
  each.
- **SC-004**: No card shows a year outside a plausible release range.
- **SC-005**: Card captions occupy the same number of lines as before, at the
  narrowest supported width.
- **SC-006**: Browsing issues the same number of source requests as before, and
  the first page appears no later than it does today.

## Credential-Safety Impact

- **Nothing new is stored.** These are facts already fetched with each catalog
  page and either discarded or rendered unreadably. No new table, no new column,
  no change to the single SQLite volume.
- **No new outbound surface.** No additional request is made to any source, and
  no new host is contacted. The genre mapping this relies on is already fetched
  for the filter menu.
- **No credentials involved.** No NAS connection, no DSM API, no stored secret is
  read or written by this change.
- **Nothing new is logged.** Titles, genres, and years stay out of logs and error
  strings exactly as they are today.

## Assumptions

- **Language and country are deferred, deliberately.** They exist today only as
  filter facets, not as per-title data; showing them would require each source
  driver to extract them per title, and it is not established that either source
  publishes them that way. They are a separate spec once that is known, rather
  than a guess bolted onto this one.
- **English is the app's vocabulary.** The interface is in English, so a Farsi
  genre on a card is treated as a defect rather than a localisation choice. This
  spec does not introduce translation as a feature; it uses a mapping the source
  already publishes.
- **The year is the source's, not ours.** No external lookup is added to fill a
  missing year. If the source does not know, neither does the card.
- **One genre is enough on a card.** The caption has a fixed budget; showing the
  first genre is more useful than showing three truncated ones.

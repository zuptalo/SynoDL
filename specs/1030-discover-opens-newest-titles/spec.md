# Feature Specification: Discover opens on the newest titles

**Feature Branch**: `feat/1030-discover-opens-newest-titles`

**Created**: 2026-09-06

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User description: "let's change the deafult sorting order to be on Release year as well so we always get the latest titles in the begining"

## Overview

Discover opens on **Most popular**, which leads with the same handful of
long-standing favourites every time. What a user wants on opening is what is new.

The obvious change — default to **Release year** — does not deliver that, and
this was measured on the live sources before choosing:

| sort, descending | what leads the first page |
|---|---|
| Release year | `Reptile Royalty 7441`, `Coco 2 2029`, `Untitled Tomb Raider Project`, `Mexicali` |
| Recently added | `Daemons of the Shadow Realm 2026`, `Jaadugar 2026`, `Oni no Hanayome 2026` |

The sources carry rows whose release year is missing or nonsense, and those sort
straight to the top of a descending year list. Spec 2006 tried excluding them
with year bounds; spec 2007 reverted that because the bound made the source's
query **ten times slower** (16–20 s against 1.9 s), so there is no cheap way to
order by year and not lead with rubbish.

**Recently added** orders by when the source published a title, which is what
"the latest titles" means in practice.

It is also the FASTEST of the three, on both sources and on the combined view
that Discover actually opens on. Measured against the live sources, three
samples each, every request on a page never fetched in that session so nothing
came from the source's cache:

| source | Most popular | Release year | Recently added |
|---|---|---|---|
| 30nama | 1272 ms | 1374 ms | **418 ms** |
| ZarFilm | 1550 ms | 1478 ms | **1322 ms** |
| both combined | 1665 ms | 3393 ms | **1364 ms** |

Three times faster on one source and ahead on the other, so the default that
shows the newest titles is also the one that opens quickest. That settles it.

A caution for anyone repeating this: an earlier run of the same measurement
showed Most popular at 120 ms on 30nama, which was the source returning pages
already fetched moments before. Timing a page twice measures the cache, not the
query — spec 2007 made the same point and it is easy to forget.

## User Scenarios & Testing

### User Story 1 - Discover opens on what is new (Priority: P1)

A user with no saved view opens Discover and sees recently published titles
first, rather than the same perennial favourites.

**Acceptance**:
1. Given an account that has never changed its sort, when Discover opens, then
   results are ordered by recently added, descending.
2. Given a source that does not offer that ordering, then the first ordering it
   does offer is used, and Discover still works.

### User Story 2 - A saved choice still wins (Priority: P1)

**Acceptance**:
1. Given a user who has chosen a sort, when they reopen Discover, then their
   choice is restored, not the new default.
2. Given a user whose saved sort IS the old default, then that saved choice is
   honoured — changing the default must not silently re-sort someone who had
   deliberately chosen it.

### Edge Cases

- A text search cannot be sorted at all by these sources; the default is
  irrelevant while a search is active and the sort control stays disabled.
- The server falls back to a default of its own for an unrecognised sort; that
  fallback and the client default must name the same ordering, or a request the
  client believes is default would come back ordered differently.

## Requirements

### Functional Requirements

- **FR-001**: The default browse sort MUST be "recently added", descending.
- **FR-002**: The server's fallback for an unrecognised sort MUST be the same
  ordering, so the two cannot disagree.
- **FR-003**: A user's saved sort MUST continue to take precedence.
- **FR-004**: Where a source does not offer the default, the existing fallback
  to its first offered ordering MUST still apply.

## Success Criteria

- **SC-001**: A new account opening Discover sees titles published recently,
  not the catalogue's perennial favourites.
- **SC-002**: No title with a missing or implausible release year leads the
  first page.
- **SC-003**: An existing user's saved sort is unchanged by the upgrade.

## Assumptions

- "Recently added" reflects when the SOURCE published a title, not when it was
  released. For a catalogue that is the better proxy for "new", because it is
  the only date the sources record reliably.

## Credential-Safety Impact

None. A default value changes; no new data, no new request, nothing stored.

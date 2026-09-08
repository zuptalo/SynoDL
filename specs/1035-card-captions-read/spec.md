# Feature Specification: Card captions read like the detail sheet

**Feature Branch**: `feat/1035-card-captions-read`

**Created**: 2026-09-07

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "the cards got better but let's improve them a bit more. In the second image — the same film — copy the details exactly for the cards. Three lines: the film's title, its type and year, its genre. Right now it's all on one line on the card and it looks like a bug."

## Overview

Spec 1032 put the year and a genre on the card, and crammed them onto the
single line that already held the rating and the type. Four unrelated facts
separated by gaps reads as a run-on rather than as information — the user's word
for it was that it looks like a bug.

The detail sheet for the same title already solves this, and solves it well:

```
The Whisper Man
★ 6.3   Movie   2026
Crime · Drama · Thriller
```

Three lines, each answering a different question: what is it called, what kind
of thing is it, and what is it like. The card adopts that SHAPE. It leads the
middle line with the rating rather than the type, which is the one place it
departs from the sheet: on a card the score is what the eye is scanning for,
and it was already the first thing there.

**This deliberately supersedes spec 1032's FR-008**, which required the caption
to occupy no more vertical space than it did before. That constraint was chosen
when the alternative was a cramped line; it produced the cramped line instead.
The user has looked at the result and asked for the height.

## User Scenarios & Testing

### User Story 1 - The card and the sheet agree (Priority: P1)

Someone scanning the grid reads a title, then what kind of thing it is and when
it was made, then what it is like — in that order. Opening it shows the same
three facts in the same order, so nothing has to be re-found.

**Why this priority**: It is the whole request, and the consistency with the
sheet is the reason to prefer this shape over any other three-line arrangement.

**Independent Test**: Compare a card against that title's detail sheet and
confirm the same three lines in the same order.

**Acceptance Scenarios**:

1. **Given** a title in the grid, **When** its card renders, **Then** the
   caption is three lines: name; rating, type and year; genres.
2. **Given** a title with more than three genres, **When** its card renders,
   **Then** only the first three are shown.
3. **Given** a title with several genres, **When** its card renders, **Then**
   the genre line lists more than one, separated the way the sheet separates
   them.
4. **Given** a title missing a fact — no rating, no year, no genre — **When**
   its card renders, **Then** the missing fact leaves no gap and no empty line.
5. **Given** every card in the grid, **When** they render, **Then** they are all
   the same height, so the grid stays aligned.

---

### Edge Cases

- **A title with none of the three facts.** The card must show just its name,
  without leaving two blank lines behind it.
- **A long genre list.** It must clip rather than wrap onto a fourth line and
  break the grid's alignment.
- **A long title.** Unchanged — it already clips to one line.
- **An active type filter.** The type is still redundant when the filter already
  says it, so it stays suppressed; that line then carries the year and rating.

## Requirements

### Functional Requirements

- **FR-001**: A card's caption MUST be three lines: the name; the type, year and
  rating; the genres.
- **FR-002**: The second line MUST read rating, then type, then year. The third
  MUST list genres and MUST NOT repeat the year.
- **FR-003**: The genre line MUST show more than one genre when the title has
  more than one, separated as the detail sheet separates them.
- **FR-004**: A line whose facts are all absent MUST NOT be rendered, and MUST
  NOT leave vertical space behind.
- **FR-005**: Every card in the grid MUST have the same height regardless of how
  many facts it has, so the grid stays aligned.
- **FR-006**: A long genre list MUST clip rather than wrap onto a further line.
- **FR-007**: The type MUST remain suppressed while a type filter is active
  (spec 1032, FR-007), since the filter already says it.

## Success Criteria

- **SC-001**: A card presents the same facts as its detail sheet, in the
  three-line shape the sheet uses.
- **SC-002**: Cards in a grid are all the same height, including titles missing
  a rating, a year or a genre.
- **SC-003**: No card's caption wraps onto a fourth line.

## Credential-Safety Impact

None. This is a rendering change over data already fetched and already shown.

## Assumptions

- **Height is now acceptable.** Spec 1032 traded it away to avoid growing the
  card; this spec spends it, at the user's explicit request, having seen both.
- **Three genres is enough on a card.** The sheet shows up to four; a card is
  narrower, and the example that prompted this shows three.

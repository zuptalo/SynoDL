# Feature Specification: Mark the version you actually downloaded

**Feature Branch**: `feat/1025-mark-version-you`

**Created**: 2026-09-04

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "currently the owned item doesn't specifically mark only the version which is downloaded and marks everything as owned which is not correct, also see how we can improve the ui for owned item so not everything is visible off the bat when it comes to options, maybe it can be an expandable list for each season and when expanded we see which one is actually downloaded, for the not owned item we probably can have the same approach with a difference that the first not downloaded season is expanded by default, and expanding another season collapses the other one so user doesn't need to scroll too much, let's also adjust the lines so we can see the whole information on smaller screens as well"

## Context

Ownership is resolved per season: if any video for season 1 is on the NAS, every
season-1 download option is stamped "Have it" — the 1080p one, the x265 one and
the other encoder's, all four. Only one of them is the file the user actually
has. The badge therefore says something untrue about three options out of four,
and it says it about exactly the options the user is choosing between.

The same screen compounds it. Every season of every quality is listed flat and
expanded, so a long-running series is a wall of near-identical rows the user must
scroll past to reach a season they do not have. And on a phone the rows overflow:
the badge is clipped mid-word and the episode list is cut off after a comma.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which version you actually have (Priority: P1)

A user opens a series they have been collecting. Season 1 is on the NAS. Today
every season-1 option claims they have it, so the badge tells them nothing about
which of the four they downloaded — and if they wanted to replace a low-quality
copy with a better one, the screen actively misleads them.

After this change the badge marks the release that matches what is on disk, and
only that one. Where the files cannot say which release they came from, no option
is marked and the season still reports what it holds.

**Why this priority**: This is a correctness problem — the app currently asserts
something false about the user's own library, which is the one thing ownership
marking exists to get right.

**Independent Test**: Put a known release of a season on the NAS, open the title,
and confirm only the matching option is marked.

**Acceptance Scenarios**:

1. **Given** season 1 on disk in a release whose resolution and release group
   match one option, **When** the title is opened, **Then** only that option is
   marked and the other season-1 options are not.
2. **Given** files whose names do not identify a release, **When** the title is
   opened, **Then** no option is marked, and the season still reports that it is
   on the NAS with its episode list.
3. **Given** two options that differ only by release group, **When** one of them
   is on disk, **Then** the other is not marked.
4. **Given** a movie already on the NAS, **Then** the same rule applies to its
   releases.
5. **Given** a season on disk, **When** no option matches it, **Then** the season
   is still reported as present — a release we cannot match is not evidence that
   the season is absent.

### User Story 2 - Reach the season you want without scrolling past the rest (Priority: P1)

The options list opens fully expanded, every season at once. For a series with
many seasons the user scrolls through seasons they already have to reach the one
they want.

After this change the options are grouped by season and collapsed, one open at a
time. A title the user does not have opens with the first season they are missing
already expanded, so the common case takes no taps at all. A title they do have
opens fully collapsed, with each season's header saying what is there.

**Why this priority**: This is the other half of the same screen, and the user
raised both together.

**Independent Test**: Open a many-season series; confirm one season is expanded,
that expanding another collapses the first, and that the pre-expanded one is the
first season not already on the NAS.

**Acceptance Scenarios**:

1. **Given** a series with several seasons and none on the NAS, **When** the title
   opens, **Then** the first season is expanded and the rest are collapsed.
2. **Given** seasons 1–2 on the NAS and 3 onwards missing, **When** the title
   opens, **Then** season 3 is expanded.
3. **Given** every season is on the NAS, **When** the title opens, **Then** all
   seasons are collapsed.
4. **Given** one season expanded, **When** the user expands another, **Then** the
   first collapses.
5. **Given** a collapsed season, **Then** its header still says whether it is on
   the NAS and how many options it has, without being expanded.
6. **Given** a movie, whose options have no seasons, **Then** the options are
   listed as they are today — grouping by a season that does not exist would add
   a layer for nothing.
7. **Given** the user expands a season and picks an option, **When** they send it,
   **Then** sending behaves exactly as it does today.

### User Story 3 - Read the whole row on a phone (Priority: P2)

On a narrow screen the option rows overflow: the badge is clipped mid-word and
the episode list is truncated after a comma, so the user cannot see what they
have or what they are choosing.

**Independent Test**: At a 360px-wide viewport, every row's badge, size, encoder
and episode list are fully readable.

**Acceptance Scenarios**:

1. **Given** a narrow screen, **When** an option row is shown, **Then** its badge
   is fully visible and never clipped.
2. **Given** a long episode list, **When** it does not fit, **Then** it wraps or is
   summarised, rather than being cut off mid-list.
3. **Given** a long release label, **Then** it wraps rather than pushing the badge
   off the row.
4. **Given** a wide screen, **Then** the rows read as they do today.

### Edge Cases

- A file named only by episode, with no release information, must not be read as
  matching whatever option happens to be listed first.
- Two options with the same resolution and no release group must not both be
  marked on the strength of a resolution match alone.
- A season present on disk but not offered by the source at all must still be
  reported by its header.
- A title whose options carry no season (a movie, or a series the source lists
  flat) must not be forced into an accordion.
- A season still downloading must read as arriving, not as owned, exactly as
  today.
- Expanding a season must not change which option is selected for sending.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A download option MUST be marked as already downloaded only when the
  release on disk can be identified as that option's release.
- **FR-002**: Identification MUST require agreement on BOTH the resolution and the
  release group. Agreement on one alone MUST NOT mark an option.
- **FR-003**: Where the files cannot identify a release, no option MUST be marked.
- **FR-004**: Failure to identify a release MUST NOT affect whether the season or
  title is reported as present.
- **FR-005**: Season presence, episode lists and the downloading state MUST behave
  exactly as they do today.
- **FR-006**: Matching MUST happen on the server. The client MUST receive only
  whether an option is already downloaded — never file names, and never the
  release tokens read from them. Nothing read from the NAS may appear in logs,
  errors or metrics.
- **FR-007**: Options for a series MUST be grouped by season, with one group
  expanded at a time.
- **FR-008**: On opening, the expanded group MUST be the first season not already
  on the NAS; when every season is present, none MUST be expanded.
- **FR-009**: Expanding a group MUST collapse any other.
- **FR-010**: A collapsed group's header MUST state whether that season is on the
  NAS and how many options it offers.
- **FR-011**: Options with no season MUST be listed without grouping.
- **FR-012**: Expanding or collapsing MUST NOT change the selected option, and
  sending MUST be unchanged.
- **FR-013**: Every row MUST be fully readable at a 360px-wide viewport: no
  clipped badge, no truncated episode list, no text pushed out of view.

### Key Entities

- **Release** — how a particular copy of a title was encoded: its resolution and
  the group that produced it. What is on disk and what an option offers are each
  described this way, and comparing them is what identifies a download.
- **Season group** — the options for one season of a series, and whether that
  season is already on the NAS.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a season on the NAS in an identifiable release, exactly one
  option is marked.
- **SC-002**: No option is ever marked on the strength of a resolution match alone.
- **SC-003**: Opening a partly-collected series takes no taps to reach the first
  season the user is missing.
- **SC-004**: At most one season's options are on screen at a time.
- **SC-005**: At a 360px viewport, no row's content is clipped or truncated.
- **SC-006**: Sending a download works exactly as it does today.

## Assumptions

- Release information comes from the file names on the NAS, which are the source's
  own release names — SynoDL saves what it downloads under the name it arrived
  with. A library filled by other means may not carry them, which is precisely the
  case FR-003 covers.
- The user confirmed the conservative reading: when the match is inconclusive, the
  season header still says it is on the NAS and no individual option is marked.
- Movies keep their current flat list; the accordion is a series affordance.

## Credential-Safety Impact

- No change to the DSM API allowlist, and no additional NAS call: the release
  tokens are read from file listings already being fetched for season presence.
- File and folder names still never leave the server and never enter a log, and
  neither do the release tokens derived from them: matching happens server-side
  and the client is told only "you already have this one". That is strictly less
  about the NAS than the episode numbers already sent today.
- Nothing new is persisted. The tokens live in the same in-memory evidence cache
  as the season presence beside them, and are rebuilt on restart.
- The UI change moves no data across the boundary; it only decides what is drawn.

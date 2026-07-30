# Feature Specification: Tasks view — posters, cleaner titles, Open in Discover

**Feature Branch**: `feat/1016-tasks-view-poster`

**Created**: 2026-07-30

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Device-testing feedback on the Tasks tab: the release year is shown twice (in the title
and on the metadata line); a task row has no poster; there's no quick way to jump from a task back to
its Discover page; and torrent-only stats show on non-torrent tasks.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cleaner task titles (Priority: P1)

A task row shows "Dexter Resurrection 2025" as the title AND "2025" again on the metadata line just
below. The duplicated year is noise. The user wants the year shown once — on the metadata line — and
removed from the title heading.

**Why this priority**: Trivial, immediate readability win with no dependencies; ships on its own.

**Acceptance Scenarios**:

1. **Given** a task whose title ends with a release year, **When** the row renders, **Then** the
   heading shows the title without the trailing year and the year appears once on the metadata line.
2. **Given** a task with no separately-known year (e.g. a manually-added download), **When** the row
   renders, **Then** the year parsed from its name is still shown on the metadata line (not lost).

---

### User Story 2 - Poster thumbnail on the task row (Priority: P1)

Task rows are a wall of text. The user wants the title's poster shown as a thumbnail on the left of
each row (as in Discover), so a download is recognisable at a glance.

**Why this priority**: The headline visual improvement. Requires the source to remember the poster
when the download is sent from Discover.

**Independent Test**: Send a download from Discover, then open Tasks — its row shows the poster
thumbnail; a task with no known poster shows a neutral placeholder, never a broken image.

**Acceptance Scenarios**:

1. **Given** a download sent from Discover after this ships, **When** its task row renders, **Then**
   the title's poster is shown as a thumbnail on the left.
2. **Given** a task with no stored poster (older or manually added), **When** its row renders, **Then**
   a neutral placeholder is shown in the same slot (no broken image, layout unchanged).

---

### User Story 3 - Open in Discover from the task detail (Priority: P2)

From a task's detail view the user wants to jump straight to that title in Discover to see its page
(plot, other qualities, related downloads).

**Why this priority**: A convenience that builds on the same stored catalog linkage as the poster.

**Independent Test**: Open the detail of a Discover-sent task; tap "Open in Discover"; the Discover
tab opens that title's page. For a task with no catalog link, the same action runs a Discover search
for the title instead.

**Acceptance Scenarios**:

1. **Given** a task with a stored catalog link, **When** the user taps "Open in Discover", **Then**
   the Discover tab opens showing that exact title's page.
2. **Given** a catalog task with no stored catalog id (older), **When** the user taps "Open in
   Discover", **Then** the Discover tab opens with a search for the task's title.
3. **Given** a task with no catalog metadata at all (plain download), **When** the detail view is
   shown, **Then** no "Open in Discover" action is offered.

---

### User Story 4 - Hide torrent-only fields for non-torrent tasks (Priority: P3)

The task detail shows upload total, upload speed, and peers/seeders. For non-torrent downloads
(HTTP/FTP/NZB/eMule) these are always zero/irrelevant and just clutter. The user wants them shown
only for torrent (bt) tasks.

**Acceptance Scenarios**:

1. **Given** a non-torrent task, **When** its detail view is shown, **Then** upload total, upload
   speed, and peers/seeders are not shown.
2. **Given** a torrent task, **When** its detail view is shown, **Then** those fields are shown as
   today.

---

### Edge Cases

- A poster URL that fails to load must fall back to the placeholder, not a broken image.
- The year strip must not mangle titles that legitimately contain a number that isn't a year, or a
  series year-range ("2022–") shown on the metadata line.
- "Open in Discover" for a stored catalog id must still work if the live Discover detail fetch later
  fails (it degrades to the source's normal needs-refresh/unavailable handling, not a crash).
- Existing stored downloads (no poster/catalog id) keep working: placeholder poster, search fallback.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The task row heading MUST show the title with the trailing release year removed.
- **FR-002**: The release year MUST still be shown once, on the row's metadata line, for both
  catalog-sent and plain tasks (using the stored year when present, else the year parsed from the
  name).
- **FR-003**: When a download is sent from Discover, the source MUST persist the title's poster URL
  and the catalog title id alongside the existing media type / year / score.
- **FR-004**: A task row MUST show the stored poster as a left-aligned thumbnail; a task without a
  stored poster MUST show a neutral placeholder in the same slot.
- **FR-005**: The task detail view MUST offer an "Open in Discover" action for tasks that carry
  catalog metadata; it MUST open the exact title when a catalog id is stored, and otherwise run a
  Discover search for the task's title. Tasks with no catalog metadata MUST NOT show the action.
- **FR-006**: The task detail view MUST show upload total, upload speed, and peers/seeders only for
  torrent (bt) tasks.
- **FR-007**: All changes MUST preserve the credential-free, allowlist-bound proxy boundary: the new
  stored fields are ordinary per-download metadata in the existing SQLite volume; no new secrets, no
  new outbound API, and the poster URL is a public CDN image already used by Discover.
- **FR-008**: New persisted columns MUST be added via append-only migrations and MUST be backward
  compatible (existing rows default to empty poster/catalog id).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a Discover-sent task, the year appears exactly once (metadata line) and the poster
  thumbnail is visible.
- **SC-002**: 100% of tasks render a poster-slot image or placeholder — never a broken image.
- **SC-003**: "Open in Discover" reaches the correct title's page for catalog-linked tasks, and a
  relevant search otherwise, in one tap.
- **SC-004**: Non-torrent task details show zero torrent-only stat rows.

## Assumptions

- The poster URL is the same public catalog image Discover already renders (`CatalogTitle.posterUrl`);
  storing it introduces no new data-sensitivity.
- Cross-tab handoff for "Open in Discover" passes the title in-app (shared catalog state), not via a
  poster/plot-laden URL query.
- Backfilling posters/ids for downloads sent before this ships is out of scope — they use the
  placeholder + search fallback.
- This is stateful (DB columns), so it is covered by Go integration tests, not the stateless e2e
  harness.

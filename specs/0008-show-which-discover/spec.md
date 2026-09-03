# Feature Specification: Show which Discover titles you already have

**Feature Branch**: `feat/0008-show-which-discover`

**Created**: 2026-09-03

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User description: "I would like SynoDL to be more aware of the contents already existing in the movie and tv-show parent folders and detect what is already downloaded there, and when the same title shows up in the Discover tab, add a badge or something which indicates the item already exists so the user doesn't by mistake download it again."

## Overview

Discover lists everything a download source carries, with no signal about what is already
sitting on the NAS. Nothing stops a user re-downloading a title they already own — which
burns the daily download allowance SynoDL itself caps per user, the NAS's bandwidth, and
disk space. For a series it is worse: the title's folder may already hold seasons 1–2, but
the download options give no hint, so the user re-sends packs they already have.

This feature makes SynoDL aware of what is already on the NAS. It reads the contents of the
movies and TV parent folders each download source is configured to write into, matches what
it finds against the titles Discover shows, and surfaces the answer in three places: a marker
on the poster while browsing, a season-by-season breakdown when opening a series, and a
confirmation before re-sending something already present — plus a way to hide owned titles
from the grid entirely.

Detection reads the folders themselves rather than SynoDL's own record of what it sent, so
content that predates SynoDL, or arrived by any other route, is recognised too.

## Clarifications

### Session 2026-09-03

- Q: How current must the reading of the NAS folders be, given that re-reading on every catalog
  page would be slow? → A: Reuse a reading for up to 5 minutes, and discard it immediately after a
  download is sent — so a title the user just sent is marked at once, and content that appeared on
  the NAS by other means is picked up within 5 minutes (FR-008, FR-010, FR-010a, SC-003a).
- Q: With owned titles hidden, a page of results can leave the grid nearly empty. Should it be
  backfilled? → A: Yes — when hiding leaves too few cards, further pages are fetched automatically
  so the grid stays full and scrolling feels unchanged (FR-023, SC-008a).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See what you already have while browsing (Priority: P1)

A user scrolls Discover looking for something to watch. Titles already on the NAS are
visibly marked, so they can skip past them instead of opening each one to find out.

**Why this priority**: This is the whole point of the request and delivers value on its own —
even with nothing else built, the user stops re-downloading by mistake.

**Independent Test**: With a title folder present on the NAS, open Discover and find that
title in the grid; it carries an "already have it" marker while its neighbours do not.

**Acceptance Scenarios**:

1. **Given** a folder for a title exists under the configured movies parent, **When** that
   title appears in the Discover grid, **Then** its card carries a clear marker saying the
   user already has it.
2. **Given** no folder for a title exists, **When** it appears in the grid, **Then** the card
   carries no such marker.
3. **Given** a title whose folder name differs from the catalog title only in case,
   punctuation, bracketed extras, a leading article, or how the year is written, **When** it
   appears in the grid, **Then** it is still recognised as present.
4. **Given** two titles that share a name but not a release year (a 1990 film and its 2017
   remake), **When** only one is on the NAS, **Then** only that one is marked.
5. **Given** a title marked as present, **When** a screen reader reads the card, **Then** the
   "already have it" state is announced, not conveyed by colour alone.
6. **Given** a title that is both unreleased and already present, **When** it renders, **Then**
   both the "Soon" marker and the ownership marker are readable and neither obscures the other.

### User Story 2 - Know which seasons of a series you already have (Priority: P2)

A user opens a series they partly own. Before choosing a download, they can see which seasons
are already on the NAS, so they only fetch the ones they are missing.

**Why this priority**: A title-level marker is misleading for series — a folder holding one
season looks identical to one holding all ten. This turns a vague signal into an actionable one.

**Independent Test**: Seed a series folder containing seasons 1 and 2, open that series in
Discover, and confirm the download options mark seasons 1 and 2 as already present and leave
the rest unmarked.

**Acceptance Scenarios**:

1. **Given** a series folder containing seasons 1 and 2, **When** the user opens that series,
   **Then** its download options show seasons 1 and 2 as already present and seasons 3+ as not.
2. **Given** a series folder that stores each season in its own subfolder, **When** the user
   opens it, **Then** the present seasons are identified correctly.
3. **Given** a series folder that stores episode files directly in the title folder, **When**
   the user opens it, **Then** the present seasons are identified correctly.
4. **Given** a movie already present, **When** the user opens it, **Then** it is shown as
   already present with no season breakdown.
5. **Given** the title's folder cannot be read, **When** the user opens the title, **Then** the
   download options are fully usable and no season markers are shown.

### User Story 3 - Guardrails against downloading it twice (Priority: P3)

A user who has decided to re-download something already present is asked to confirm first, and
a user who never wants to see owned titles can hide them from the grid.

**Why this priority**: The marker already prevents most mistakes; these close the remaining gap
and make long browsing sessions cleaner. Neither is needed for the first two stories to be useful.

**Independent Test**: Send a title that is already present and confirm a prompt appears that can
be cancelled; separately, enable the hide control and confirm owned titles leave the grid and the
setting survives a reload.

**Acceptance Scenarios**:

1. **Given** a title already present, **When** the user sends it, **Then** they are asked to
   confirm before anything is sent.
2. **Given** that confirmation, **When** the user cancels, **Then** nothing is sent and no
   allowance is consumed.
3. **Given** that confirmation, **When** the user accepts, **Then** the download proceeds
   exactly as it does today.
4. **Given** a series where only the selected season is already present, **When** the user sends
   that season, **Then** they are asked to confirm.
5. **Given** a title not present, **When** the user sends it, **Then** no confirmation appears.
6. **Given** the hide-owned control is enabled, **When** the grid renders, **Then** titles
   already present are absent and scrolling for more results keeps working.
7. **Given** the hide-owned control was enabled, **When** the user returns later or signs in on
   another device, **Then** it is still enabled.

### Edge Cases

- A title's folder exists but is empty, or holds a partial download — treated as already
  present, since the point is to stop a second download of the same thing.
- Two different catalog titles reduce to the same folder name — both are marked present. This
  mirrors how sending already behaves (both would share one destination folder).
- A source has its movies and TV parents pointing at the same folder.
- A configured parent folder does not exist on the NAS, or the account cannot read it.
- A parent folder holds thousands of titles.
- Titles and folder names in a non-Latin script, or mixing scripts.
- A still-running series whose catalog title carries an open-ended year range ("2019 –").
- A title present under a parent belonging to a different configured source.
- The user has no permission to write to the parent folder — they still see the ownership
  marker, but sending remains blocked by the existing folder rules.
- The same title is offered by two configured sources — each listing is marked independently.

## Requirements *(mandatory)*

### Functional Requirements

**Detecting what is already there**

- **FR-001**: The system MUST determine, for each title shown in Discover, whether a folder for
  that title already exists under the movies or TV parent folder configured for download sources.
- **FR-002**: Detection MUST read the actual contents of those folders, so content that arrived
  on the NAS by any route — including before SynoDL was installed — is recognised.
- **FR-003**: A movie MUST be looked for under the movies parent, and a series or anime under
  the TV parent.
- **FR-004**: Matching MUST disregard differences in letter case, punctuation, spacing,
  surrounding brackets, a leading article, and how a release year is written or bracketed.
- **FR-005**: When both the folder name and the catalog title carry a release year, those years
  MUST agree for the title to be considered present. When either carries no year, the name match
  alone stands.
- **FR-006**: Matching MUST work for titles written in non-Latin scripts.
- **FR-007**: When several sources are configured, the folders examined MUST be the union of the
  parents across enabled sources, with each distinct folder examined once.
- **FR-008**: A title downloaded through SynoDL MUST be recognised as present on the very next
  catalog request after it is sent, without restarting or reinstalling the app.
- **FR-008a**: When an administrator changes a source's parent folders, or adds, disables, or
  removes a source, any retained reading MUST be discarded, so ownership is never reported from
  folders that are no longer the configured ones.
- **FR-009**: If the NAS is unreachable, or a parent folder is missing or unreadable, the system
  MUST report nothing as present and Discover MUST continue to browse, search, and send exactly
  as it does today. A failed read MUST NOT surface an error to the user.
- **FR-010**: Browsing MUST NOT wait on a fresh reading of the folders. A reading taken within the
  last 5 minutes MAY be reused as-is; a reading older than that MUST be refreshed before it is used
  again.
- **FR-010a**: Content that appears on the NAS by any route other than SynoDL MUST be recognised as
  present within 5 minutes of appearing, with no user action required.

**Marking titles in the grid**

- **FR-011**: A title detected as present MUST carry a clear visual marker on its card in the
  Discover grid.
- **FR-012**: That marker MUST NOT convey its meaning by colour alone and MUST be announced to
  assistive technology.
- **FR-013**: The marker MUST coexist legibly with the existing "unreleased" marker.

**Season detail for series**

- **FR-014**: Opening a series or anime that is present MUST show which seasons are already on
  the NAS, alongside that title's download options.
- **FR-015**: Season presence MUST be detected both when seasons are stored in their own
  subfolders and when episode files sit directly in the title's folder.
- **FR-016**: Where a count of episodes present is available it MAY be shown; the season presence
  markers MUST NOT depend on a count being available.
- **FR-017**: If the title's folder cannot be read, the download options MUST remain fully usable
  with no season markers shown.
- **FR-018**: A movie that is present MUST be shown as present without a season breakdown.

**Guardrails**

- **FR-019**: Sending a title that is present — or, for a series, a season that is present — MUST
  ask the user to confirm before anything is sent.
- **FR-020**: Cancelling that confirmation MUST send nothing and consume none of the user's
  download allowance.
- **FR-021**: No confirmation MUST appear when the title or selected season is not present.
- **FR-022**: Discover MUST offer a control that hides titles already present from the grid.
- **FR-023**: With that control enabled, hidden titles MUST NOT appear and loading further
  results MUST continue to work.
- **FR-023a**: When hiding owned titles leaves too few results to fill the grid, further results
  MUST be fetched automatically until the grid is filled or the catalog is exhausted, so the user
  never sees a sparse or empty grid merely because they own much of what was returned.
- **FR-024**: The state of that control MUST be remembered for the user and restored on their
  next visit, including on another device.

**Boundaries**

- **FR-025**: The ownership signal MUST report only whether a browsed title is present and, for a
  series, which seasons — never a listing of the parent folder's contents, and never a folder path.
- **FR-025a**: A title supplied by a client when asking about ownership MUST NOT be able to address
  anything outside the configured parent folder. Any value that would escape that folder — through
  path separators, parent-directory references, or an absolute path — MUST be rejected as a bad
  request rather than sanitised into a different folder and answered.
- **FR-025b**: Ownership lookups MUST be bounded per user, so a signed-in client cannot use the
  feature to generate unlimited work against the operator's NAS.
- **FR-025c**: An ownership lookup MUST NOT reveal more about a title than the user could already
  learn by opening that title's details. Where a user's catalog is narrowed by a content rating,
  the ownership lookup MUST NOT be a way around that narrowing that opening the title is not.
- **FR-026**: Folder and file names read from the NAS MUST NOT appear in logs, error payloads,
  metrics, or panics.
- **FR-027**: Detection MUST NOT change what a user is permitted to download; existing per-user
  folder permissions continue to govern sending.

### Key Entities

- **Library entry** — one title folder observed under a configured parent: its name, the parent
  it sits under, and the release year embedded in its name when there is one.
- **Library snapshot** — the set of library entries observed across the configured parents at one
  point in time, used to answer "do I have this?" while browsing.
- **Season presence** — for one series folder, which seasons are already on the NAS and, where
  known, how many episode files each holds.
- **Hide-owned preference** — a per-user setting recording whether owned titles are hidden from
  the Discover grid.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user scrolling Discover can tell which titles they already have without opening
  any of them.
- **SC-002**: Content that arrived on the NAS before SynoDL was installed, or by any route other
  than SynoDL, is marked as already present.
- **SC-003**: A title downloaded through SynoDL is marked as present the next time the user
  browses Discover, within the same sitting.
- **SC-003a**: A title added to the NAS outside SynoDL is marked as present within 5 minutes,
  without the user reloading or signing in again.
- **SC-004**: No title is ever marked present when it is not — verified against same-name
  different-year pairs and near-miss names.
- **SC-005**: With the NAS unreachable, Discover browses, searches, and sends exactly as before,
  with nothing marked present and no error shown.
- **SC-006**: Opening a partly-owned series shows which seasons are present before the user
  chooses what to download.
- **SC-007**: A user re-sending something already present is asked to confirm first and can back
  out with nothing sent.
- **SC-008**: Hiding owned titles removes them from the grid, and the choice survives a reload
  and follows the user to another device.
- **SC-008a**: With owned titles hidden, the grid still fills with results and scrolls continuously,
  even when the user already owns most of what the catalog returns.
- **SC-009**: Opening and scrolling Discover feels no slower than before the feature.

## Credential-Safety Impact

Required by constitution Principle III.

- **What is read from the NAS, and why it is inside the boundary.** Listing folders under the
  operator's configured parents is already how the destination picker works, and reading a title
  folder's contents uses the same allowlisted file-listing capability with a different filter.
  No additional NAS API is authorized, no new host is contacted, and the target remains the
  single operator-configured NAS. This still widens what SynoDL reads from the NAS — from
  directory names only, to the names of files inside a browsed title's folder — which is why
  this section and a checklist review are required.
- **What is stored.** Nothing about the NAS's contents is persisted. The snapshot used to answer
  "do I have this?" is held only in memory for the life of the process and rebuilt on demand, so
  no migration, no second datastore, and no durable copy of the user's library exists. The only
  new persisted value is a per-user on/off flag for hiding owned titles, which carries no secret.
- **What is exposed, and to whom.** The snapshot is derived from the operator's own source
  configuration and is therefore identical for every user of the instance; it is not scoped per
  user. What reaches a client is only a yes/no per title the user is already browsing, plus the
  seasons of a title they have opened — never a directory listing, never a path outside the
  configured parents. A user who cannot write to a parent folder may still learn that a title
  exists there; this is deliberate, since it is the same operator-level fact the ownership marker
  exists to communicate, and it grants no new ability — sending remains governed by the existing
  per-user folder permissions (FR-027).
- **What a client may influence.** The only client-controlled input reaching a NAS path is the
  title being asked about, and it names a folder *inside* an operator-configured parent — never the
  parent itself, never a host, never an absolute path. Input that would escape the parent is
  refused outright rather than corrected (FR-025a), so there is no sanitising step whose output a
  caller could steer. Lookups are additionally rate-bounded per user (FR-025b) so an authenticated
  client cannot turn the feature into an amplifier against the operator's own NAS.
- **What could appear in logs or errors.** Folder and file names read from the NAS are content,
  and MUST NOT reach log lines, error payloads, metrics, or panics (FR-026). Failures to read are
  swallowed into "nothing is present" rather than surfaced (FR-009), so a permissions or
  connectivity problem cannot leak a path through an error message shown to a user.
- **Why.** The feature's whole value comes from reading the NAS's contents, which is a genuinely
  wider read than SynoDL has taken before. Keeping the result in memory, scoping it to the
  operator's configured parents, and refusing to log any of it keeps that widening as narrow as
  the feature allows.

## Assumptions

- SynoDL already names each destination folder after the catalog title it came from, so content
  it downloaded matches exactly and the normalisation rules exist to catch everything else.
- "Already have it" means a folder for the title exists — not that the download finished or that
  the files are complete or of any particular quality. An in-progress or abandoned download
  therefore counts as present, which is the desired behaviour: it stops a second attempt.
- Parent folders are operator configuration. A source with no parents configured contributes
  nothing to detection, and its titles are simply never marked.
- Download sources embed the release year at the end of a title, which is what makes the
  year-agreement rule in FR-005 possible.
- Season numbers can be recovered from season subfolder names and from episode file names in the
  conventional forms those sources produce.

## Out of Scope

- Verifying that what is on the NAS is complete, playable, or of a particular quality — presence
  is the only question asked.
- Episode-level marking. Season is the finest granularity offered.
- Any change to how the catalog lists titles, including the existing behaviour where a title
  carried by two sources is listed once per source.
- Managing, renaming, moving, or deleting existing library content.
- Recognising a title that has been renamed on the NAS beyond what the matching rules cover.
- Closing the **pre-existing** gap in content-rating enforcement. A user's content rating narrows
  what the catalog *search* returns, but retrieving one title's details by its id is not narrowed
  today. FR-025c holds ownership lookups to that same existing line — it neither widens the gap nor
  closes it. Closing it properly means enforcing the rating per title across every title-addressed
  route, which is its own change and belongs in its own spec.

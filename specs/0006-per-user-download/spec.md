# Feature Specification: Per-User Download Statistics and Richer Notifications

**Feature Branch**: `feat/0006-per-user-download`

**Created**: 2026-07-29

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "When sending notifications about downloads, use the human readable name in the push notification; for the admins and owner who subscribe to know about other users' activity also mention the username of whoever added the task. Add statistics so admins and owner can see how many movies and TV shows each user has downloaded over time and the average size for their downloaded movies and series and overall — noting that even paused or canceled downloads count toward the user's daily limit since 30nama counts them as well. Show a historical graph of number of downloads per day / week / month / year / all-time. The graphs and statistics live under their own Statistics section in Settings."

## Clarifications

### Session 2026-07-29

- Q: When a whole season is sent at once, does that count as one download or one per episode? → A: Per episode/file — one download per file, matching the existing daily-limit accounting so statistics and limit usage always agree.
- Q: The catalog classifies sends as movie, series, or anime — how should anime be bucketed? → A: Its own bucket — three media buckets (movie / series / anime), with a combined total still available.
- Q: Should the new download history be seeded from existing (incomplete, size-less, folder-deduplicated) records? → A: Start fresh — history begins at rollout so every displayed figure is accurate; no approximate backfill.
- Q: What happens to a user's download history when their account is deleted? → A: Deleted with the account, consistent with existing session cascade behavior.
- Q: Should directly-added (torrent/URL) downloads be tracked too, not just catalog ones? → A: Yes — track them with their adding user as well, recorded against a separate source so catalog and direct activity can be viewed apart or combined. (Supersedes the earlier catalog-only statistics scope.)
- Q: Directly-added downloads carry no catalog metadata — how are they categorized into movie / TV / music video / music? → A: Auto-classify from destination folder and file extension, with the user able to choose or correct the category when adding the task.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Readable, attributed download notifications (Priority: P1)

A household member receives a push notification when a download they started
completes. Instead of a cryptic release-scene file name, the notification shows
the same clean, human-readable title they see in the app (e.g. the show or movie
title with the episode marker). An admin or the owner — who has chosen to be
notified about *everyone's* activity — additionally sees **which user** added the
download, so they can tell at a glance whose activity it was.

**Why this priority**: It is the most immediately visible improvement, is
independent of the statistics work, and directly addresses the top request. It
reuses existing notification plumbing and existing ownership attribution.

**Independent Test**: Add a Discover download as a regular user, let it complete,
and confirm the completing device receives a notification whose body is the clean
title (not the raw file name). Repeat with an admin subscribed to all activity
and confirm the notification also names the user who added it. A regular user
never sees another user's name (they are only notified about their own).

**Acceptance Scenarios**:

1. **Given** a download whose raw file name is a release-scene string, **When** it
   completes and a notification is sent, **Then** the notification body shows the
   human-readable title (title plus episode marker when it is a series) rather
   than the raw file name.
2. **Given** an admin (or the owner) who is subscribed to all users' activity,
   **When** another user's download triggers a notification, **Then** the
   notification identifies the user who added it.
3. **Given** a regular (non-admin) user, **When** they receive a notification,
   **Then** it concerns only their own downloads and never reveals another user's
   name.
4. **Given** any notification, **When** it is generated, **Then** it contains no
   credentials, session ids, one-time codes, or full download URIs.

---

### User Story 2 - Per-user download statistics in Settings (Priority: P2)

An admin or the owner opens a new **Statistics** section in Settings and sees, per
user, how many movies, how many series episodes, and how many anime episodes that
user has downloaded, and the average file size for each bucket plus overall. A
regular user opening the same section sees only their *own* figures. This lets the
operator understand who is downloading what and how much space it tends to use.

**Why this priority**: This is the core insight the operator asked for. It
depends on newly persisted, durable per-download history, so it follows the
notification slice but is the primary analytical payoff.

**Independent Test**: With several completed Discover downloads of known types and
sizes attributed to known users, open the Statistics section as an admin and
confirm each user's movie count, series count, and average sizes match the
underlying downloads. Open it as a regular user and confirm only their own totals
appear and no other user is listed.

**Acceptance Scenarios**:

1. **Given** a set of completed Discover downloads with known types and sizes,
   **When** an admin opens the Statistics section, **Then** each user's movie
   count, series count, anime count, the average size for each of those buckets,
   and the overall average size are shown and match the underlying data.
5. **Given** a single send containing multiple episodes, **When** counts are
   computed, **Then** each episode counts as one download (consistent with how
   the daily limit already counts them).
2. **Given** a regular user, **When** they open the Statistics section, **Then**
   they see only their own statistics and no other user appears.
3. **Given** downloads that are still in progress, paused, or canceled, **When**
   average sizes are computed, **Then** those downloads are excluded from the size
   averages (because no real file size is known for them) while remaining counted
   in download counts.
4. **Given** a user with no completed downloads of a given type, **When** their
   average size for that type is displayed, **Then** it is shown as not-available
   rather than as zero or an error.

---

### User Story 3 - Historical downloads graph over time (Priority: P3)

Within the Statistics section, the viewer sees a historical graph of the number of
downloads over time and can switch the bucket between per day, per week, per
month, per year, and all-time. Admins/owner can view this for any user (or all
users); a regular user sees only their own history. This reveals trends — busy
periods, growth, and general download habits.

**Why this priority**: It is the richest visualization and builds on the same
persisted history as US2, but it is additive: US2 already delivers value without
the time-series chart.

**Independent Test**: With downloads spread across several days/weeks, open the
graph and switch buckets; confirm each bucket's bar/point reflects the correct
count of downloads in that period, that switching granularity re-buckets the same
underlying downloads correctly, and that the counts here include paused/canceled
downloads (count-based, consistent with the daily-limit accounting).

**Acceptance Scenarios**:

1. **Given** downloads spread over time, **When** the viewer selects a "per day"
   bucket, **Then** the graph shows one value per day equal to the number of
   downloads added that day.
2. **Given** the same data, **When** the viewer switches to per week / month /
   year / all-time, **Then** the same downloads are re-aggregated into the chosen
   bucket without a page reload and totals remain consistent.
3. **Given** an admin viewing the graph, **When** they select a specific user,
   **Then** the graph reflects only that user's downloads; **When** they select
   all users, **Then** it reflects the combined total.
4. **Given** a period with no downloads, **When** the graph renders, **Then**
   empty buckets are shown as zero rather than omitted, so gaps are visible.
5. **Given** both catalog and direct downloads, **When** the viewer changes the
   source filter, **Then** the graph re-aggregates to catalog-only, direct-only,
   or all-sources accordingly.

---

### User Story 4 - Track directly-added downloads by source (Priority: P2)

Not everything comes from the catalog. A user adds a torrent or a direct URL from
the task list — a movie, a TV episode, a music video, or a music file. The system
records who added it and what it is (auto-detected from the folder and file type,
which the user can correct as they add it), tagged as a **direct** download so it
stays distinct from catalog activity. In the Statistics section the viewer can look
at catalog downloads on their own, direct downloads on their own, or everything
combined across all sources.

**Why this priority**: It broadens statistics to the operator's real usage (many
downloads are manual) and is the same magnitude of value as US2. It shares the
history/persistence and aggregation machinery with US2, so it is built alongside
it rather than after.

**Independent Test**: Add a torrent/URL from the task list, optionally correct its
category, let it complete. Confirm it appears under the direct source attributed to
the adding user with the chosen/detected category and its real size, that it does
NOT appear under catalog figures, that it does NOT count against the daily limit,
and that the combined view sums catalog and direct together.

**Acceptance Scenarios**:

1. **Given** a user adds a download directly from the task list, **When** it is
   recorded, **Then** it is attributed to that user and tagged as a direct
   download, separate from catalog downloads.
2. **Given** a directly-added download, **When** its category is not explicitly
   chosen, **Then** it is auto-classified from the destination folder and file
   type; **When** the user chooses a category at add time, **Then** that choice is
   used instead.
3. **Given** both catalog and direct downloads exist, **When** the viewer switches
   the source filter, **Then** the statistics and graph show catalog-only,
   direct-only, or the combined all-sources totals accordingly.
4. **Given** a directly-added download, **When** the daily limit is evaluated,
   **Then** the direct download does not count against it (the limit remains
   catalog-only).

---

### Edge Cases

- **Missing or ambiguous title**: when a download has no clean title derivable
  (e.g. a bare file with no recognizable folder/episode structure), the
  notification falls back to a sensible readable form rather than an empty body.
- **Media type boundary**: a download's type comes from the catalog send and is
  recorded as one of movie, series, or anime. Downloads with no recorded type are
  excluded from type-specific counts but still appear in overall totals.
- **Size never observed**: a download that is added but never completes (paused
  indefinitely, canceled, failed, or removed before completion) contributes to
  counts but has no size and is excluded from size averages permanently.
- **Re-download to the same destination**: sending the same title again is a
  distinct download event for counting and (if it completes) a distinct size
  sample; history is append-only and is not overwritten by a re-send.
- **Admin reset of a user's daily count**: an operator resetting a user's daily
  usage affects the daily-limit window but MUST NOT retroactively erase the user's
  long-term statistics history.
- **Very large or zero-byte reported sizes**: size averages tolerate outliers and
  do not crash or display misleading values (e.g. an obviously bogus zero size is
  handled gracefully).
- **Directly-added downloads**: these carry no catalog metadata, so their category
  is inferred from destination folder and file type and may be corrected by the
  user at add time. They are recorded under the **direct** source and never mixed
  into catalog figures unless the viewer selects the combined view.
- **Unattributable direct download**: if a directly-added download cannot be tied
  to a user (e.g. it was created outside the app on the NAS itself), it MUST NOT be
  attributed to an arbitrary user; it is either excluded or shown as unattributed,
  never counted against someone else.
- **Mixed-content or ambiguous folder**: a direct download whose folder and file
  type disagree (e.g. an audio file in a movies folder) falls back to the user's
  explicit choice when given, otherwise to **other**, rather than guessing wrongly
  and silently skewing a category.

## Requirements *(mandatory)*

### Functional Requirements

**Notifications**

- **FR-001**: Download notifications MUST present a human-readable title (the same
  clean title shown in the task list, including the season/episode marker for
  series) instead of the raw Download Station file name.
- **FR-002**: When the notified subscriber is an admin or the owner receiving
  notifications about all users' activity, the notification MUST also identify the
  user who added the download.
- **FR-003**: A non-admin user MUST only ever be notified about their own
  downloads and MUST NOT be shown any other user's name.
- **FR-004**: Notifications MUST NOT contain credentials, session identifiers,
  one-time codes, or full download URIs.

**Download history & statistics data**

- **FR-005**: The system MUST durably record, for every tracked download, the user
  who added it, the **source** it came from (catalog vs direct), the media
  category, the time it was added, and — once the download completes — its real
  file size.
- **FR-006**: The download history MUST be append-only: re-downloading the same
  title creates a new record and does not overwrite prior history.
- **FR-007**: The system MUST backfill a download's real file size when the
  download completes; downloads that never complete MUST remain size-less.
- **FR-008**: The system MUST record one history entry per downloaded file (so a
  multi-episode send produces one entry per episode), consistent with how the
  daily limit already counts them.
- **FR-009**: The system MUST record directly-added downloads (torrent/magnet/URL
  added from the task list) against the user who added them, using the existing
  ownership attribution.
- **FR-010**: Every history entry MUST carry a source of either **catalog**
  (Discover) or **direct** (manually added), so the two can be reported separately
  and combined.
- **FR-011**: History MUST begin at rollout; the system MUST NOT seed statistics
  from pre-existing incomplete records.
- **FR-012**: When a user account is deleted, that user's download history MUST be
  deleted with it.

**Media categories**

- **FR-013**: Catalog downloads MUST take their media category from the catalog
  send: **movie**, **series**, or **anime**.
- **FR-014**: Direct downloads MUST be auto-classified into a media category
  (**movie**, **series**, **anime**, **music video**, **music**, or **other**)
  from the destination folder and file type.
- **FR-015**: Users MUST be able to choose or correct the media category when
  adding a download directly; an explicit choice overrides the auto-classification.
- **FR-016**: Downloads whose category cannot be determined MUST be recorded as
  **other** and still counted in totals.

**Statistics presentation**

- **FR-017**: The system MUST provide, per user, a count of downloads per media
  category.
- **FR-018**: The system MUST provide, per user, the average file size per media
  category and an overall average, computed only from completed downloads with a
  known real size.
- **FR-019**: Statistics MUST be viewable **per source** (catalog only, direct
  only) and **combined across all sources**.
- **FR-020**: The system MUST provide a time-bucketed count of downloads per user
  (and in aggregate) with selectable granularity of day, week, month, year, and
  all-time, including empty buckets as zero within the covered range.
- **FR-021**: A new **Statistics** section MUST be reachable from Settings.
- **FR-022**: A regular user MUST see only their own statistics; an admin or the
  owner MUST be able to see every user's statistics (per user and/or aggregate).
  This visibility scope MUST be enforced server-side (derived from the session's
  role), so a crafted client request cannot widen a non-admin's scope.
- **FR-023**: Average sizes MUST count only completed downloads; counts MUST
  include paused and canceled downloads.
- **FR-024**: When a statistic has no data (e.g. no completed movies for a user),
  the system MUST present it as not-available rather than zero or an error.

**Daily-limit behavior (affirmed, unchanged)**

- **FR-025**: The per-user daily download limit MUST continue to count every
  catalog download at the time it is added, including downloads that are later
  paused or canceled, matching the source's own accounting.
- **FR-026**: This feature MUST NOT change the scope of the daily limit: it
  continues to govern catalog downloads only. Recording direct downloads for
  statistics MUST NOT cause them to count against the limit.

### Key Entities *(include if feature involves data)*

- **Download record (history)**: one durable, append-only entry per downloaded
  file. Attributes: the user who added it, the source (catalog / direct), the media
  category (movie / series / anime / music video / music / other), the time it was
  added, completion state, and the real file size once completed. Deleted together
  with its user. This is the source of truth for all statistics.
- **User statistics summary**: a per-user aggregation derived from download
  records — per-category counts and average sizes plus an overall average,
  available per source and combined.
- **Download time-series**: counts of download records grouped into a chosen time
  bucket (day / week / month / year / all-time), per user or aggregate, filterable
  by source.
- **Notification content**: the readable title of the download plus, for
  all-activity subscribers, the attributed username.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of download notifications for downloads that have a derivable
  title display that human-readable title rather than the raw file name.
- **SC-002**: Every notification sent to an all-activity admin/owner subscriber
  for another user's download names the responsible user; 0% of notifications sent
  to non-admin users reveal another user's name.
- **SC-003**: For any user, the per-category counts and average sizes shown in
  Statistics match the underlying completed downloads exactly (verifiable against
  seeded data).
- **SC-004**: Switching the historical graph between day/week/month/year/all-time,
  and between catalog / direct / all-sources, re-aggregates the same downloads with
  consistent totals and updates without a full page reload.
- **SC-008**: Directly-added downloads appear only under the direct source (and the
  combined view), never in catalog-only figures, and never count against the daily
  limit — verifiable with a mix of catalog and direct seeded downloads.
- **SC-005**: A regular user can never view another user's statistics through the
  Statistics section (0% cross-user leakage).
- **SC-006**: Paused and canceled downloads are reflected in download counts and in
  daily-limit usage, and are excluded from size averages — verifiable with a mix of
  completed and canceled seeded downloads.
- **SC-007**: The Statistics section loads and renders its summary and graph
  quickly enough to feel instant on a typical household library (hundreds to low
  thousands of download records).

## Assumptions

- **Statistics cover both catalog and directly-added downloads**, recorded under
  distinct sources so they can be read apart or combined (settled with the
  requester; supersedes an earlier catalog-only scope).
- **Direct downloads are categorized heuristically with a user override** at add
  time, since no catalog metadata exists for them (settled with the requester).
- **Average sizes use real, completed file sizes only.** Because a download's true
  size is only known once it finishes, the completion watcher backfills size;
  paused/canceled downloads count toward counts and the daily limit but have no
  size and are excluded from averages (settled with the requester).
- **The historical graph is rendered as lightweight in-app visuals with no new
  third-party charting dependency**, consistent with the project's
  minimal-dependency posture (settled with the requester).
- **The daily-limit scope is unchanged** — Discover-only — and its existing
  "paused/canceled still count" behavior is preserved, not redefined, by this
  feature (settled with the requester).
- **Roles are the existing model**: the owner is the first/protected admin;
  admins have all-activity visibility, regular users are limited to their own
  data. No new role tier is introduced.
- **Attribution already exists**: the system already knows which user added each
  Discover download; this feature consumes that attribution rather than
  re-deriving it.
- **This is stateful-mode behavior**: statistics and durable history apply when the
  server runs in its stateful, multi-user mode; the credential-safety invariants
  (no secrets/sids/OTP/full URIs in logs or payloads) continue to hold.

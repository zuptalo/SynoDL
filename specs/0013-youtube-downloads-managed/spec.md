# Feature Specification: YouTube downloads you can watch, keep, and retry

**Feature Branch**: `feat/0013-youtube-downloads-managed`

**Created**: 2026-09-08

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Let's make the youtube download jobs persisted like the other jobs as well with the thumbnail and title and link and information around the lyric and language and possibility to show the live progress bar for each one and also possibility for retrying download in case one fails, let's also in case of channel and playlist make sure that our link gets expanded to individual items in that channel or playlist, let's make sure no more than 4 parallel YouTube downloads can happen at any given time, the rest should get in the queue and get started as the ongoing ones reach the final state, it being failed or downloaded, tapping each item should open up a view with the complete detail around that, creation and final state timestamp in 24:00 format and down to second, dates should be shown in swedish style like 2026-11-21 format instance, also double check and make sure that we are embedding the thumbnails and in case of channel and playlist if the name of the artist is not obvious let's use the channel name or playlist name as the artist and album info in the downloaded items, just like tasks users should see their own tasks and admins should see everyone's"

## Overview

Spec 0012 shipped YouTube downloads as deliberately thin things. Paste a link,
pick Music or Music video, and a worker goes away and does it — with four states,
no progress, no history, and no way back in if it went wrong. That was the right
first cut: the worker had no channel back to SynoDL, so anything richer would
have been invented rather than reported.

This spec turns them into downloads the user actually manages. A download now
keeps a record of itself, so last week's is still there after the cluster has
long since swept the job that did it. It reports genuine progress while it runs,
read from what the worker itself says it is doing. It can be opened to see
everything known about it — what it is, who published it, whether lyrics landed
and in which language, when it started and when it finished. A failed one can be
retried on purpose. A playlist or channel stops being one opaque bulk job and
becomes the individual items it contains, each with its own row, its own
progress, and its own outcome. Only four run at once, and the rest wait their
turn rather than all piling onto the cluster.

And it closes a defect: today every signed-in user sees every user's YouTube
downloads. They should follow the same rule as NAS tasks — your own, unless you
are an admin.

## Relationship to Spec 0012

This spec deliberately supersedes four decisions 0012 made, each of which was
correct given what was knowable then. They are recorded here rather than quietly
reversed:

| 0012 decision | Superseded by | Why it changes |
|---|---|---|
| **FR-017** — exactly four states, no progress, no rate, no estimate | FR-010, FR-011 | 0012 noted the worker "has no channel back to us". It does: a worker's own output is readable, and that output can be a format SynoDL defines rather than text it scrapes. |
| **Assumption** — "concurrency is bounded by a small operator-invisible limit, tuned in code", with no server-side queue | FR-021 – FR-025 | A cluster-bounded limit means excess requests sit Pending as far as the cluster is concerned, which the user cannot see or reorder. An explicit, durable queue of 4 is visible, survives restart, and is the operator's to tune. |
| **FR-024** — only *failures* get a durable record; a success fades with the orchestrator's cleanup window | FR-001 – FR-005 | "The saved files are its record" holds for the media server, but not for the user asking what they downloaded last Tuesday. |
| **FR-014 / FR-015** — a channel or playlist is one bulk request, deduplicated by the library's own archive file | FR-014 – FR-020 | A single bulk row cannot show which item is downloading, cannot report which item failed, and cannot be retried at item granularity. |
| **FR-024a** as implemented — dismissal is refused while a download is running | FR-005a – FR-005c | With a queue and with groups of hundreds, refusing dismissal leaves no way to call off work already started. Dismissal now forgets the record and cancels what has not started, while still never stranding a running worker. |

Three 0012 decisions explicitly **stand**:

- **FR-021** — nothing retries automatically. Retry in this spec is always a
  deliberate user action.
- **FR-005** — the download never runs inside the SynoDL server process.
- The Principle III rule that in-flight worker state is derived from the
  orchestrator, never mirrored. What this spec stores is the **request** and its
  **finished outcome**; while a worker is alive, what it is doing is still read
  from the orchestrator and the worker, not from a mirror.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - My downloads are mine (Priority: P1)

Someone signs in and looks at their downloads. They see the YouTube downloads
they submitted, and nobody else's. An admin looking at the same list sees
everyone's, each marked with who asked for it — exactly as the list already
behaves for NAS tasks.

**Why this priority**: This is a live defect, not an enhancement. Today the list
returns every user's YouTube downloads to every signed-in user, with only the
"added by" name gated behind admin. It is small, self-contained, and shippable on
its own.

**Independent Test**: Submit downloads as two different users, then confirm each
sees only their own, and that an admin sees both with attribution.

**Acceptance Scenarios**:

1. **Given** two users have each submitted a YouTube download, **When** the first
   signs in and opens the list, **Then** only their own download is shown.
2. **Given** the same two downloads, **When** an admin opens the list, **Then**
   both are shown, each labelled with the user who submitted it.
3. **Given** a download belonging to another user, **When** a non-admin tries to
   open, retry, or dismiss it, **Then** the request is refused and nothing about
   that download is revealed.

---

### User Story 2 - What I downloaded is still there next week (Priority: P1)

A user comes back days later and can still see what they asked for and how it
turned out — the artwork, the title, who published it, the link, whether it was
music or a music video, when they asked, and when it finished. The worker that
did the work is long gone; the record is not.

**Why this priority**: Everything else in this spec — the detail view, retry, the
queue, per-item expansion — needs a durable record of a request to hang off. It
is the foundation, and on its own it already answers "did that actually work?".

**Independent Test**: Submit a download, let it finish, remove the worker job
entirely, and confirm the download is still listed with its title, artwork, link,
and both timestamps intact.

**Acceptance Scenarios**:

1. **Given** a download that completed, **When** the orchestrator's record of its
   job has been swept, **Then** the download is still listed with its title,
   publisher, artwork, link, mode, and outcome.
2. **Given** a download that failed, **When** the user looks days later, **Then**
   it is still listed as failed with its reason.
3. **Given** downloads that are running and downloads that finished, **When** the
   server restarts, **Then** every finished one is still recorded and every
   running one still reports its real state.
4. **Given** a download record, **When** the user dismisses it, **Then** it is
   removed from their history and the files it saved are untouched.
5. **Given** an expanded group with items saved, queued and running, **When** the
   user dismisses the group, **Then** the group and all its item records go, the
   queued items never start, the running ones are left to finish, and every file
   already saved remains on the NAS.

---

### User Story 3 - Watch a download actually progress (Priority: P1)

Instead of a row that says only "downloading", the user sees how far along it is
— a progress bar that moves. When it finishes they can see whether lyrics were
saved alongside it, and in what language.

**Why this priority**: This is the single most-felt gap in 0012. A long download
with no progress is indistinguishable from a stuck one, which is exactly the
anxiety this removes.

**Independent Test**: Start a download against a worker that reports progress and
confirm the row's progress advances and then reaches its finished state, and that
the detail view reports whether lyrics were written and their language.

**Acceptance Scenarios**:

1. **Given** a download whose worker is running, **When** the user watches the
   row, **Then** a progress indicator advances as the worker reports progress.
2. **Given** a download whose worker has not reported progress yet, **When** the
   user looks, **Then** the row shows that it is working without inventing a
   percentage.
3. **Given** a finished download whose source published captions in its original
   language, **When** the user opens it, **Then** it reports that lyrics were
   saved and names the language.
4. **Given** a finished download whose source published no captions, **When** the
   user opens it, **Then** it says so plainly, and the download is still reported
   as successful.
5. **Given** a worker whose output cannot be read at all, **When** the user
   watches the row, **Then** the download still reports its correct life state
   and simply shows no percentage.

---

### User Story 4 - Open one and see everything about it (Priority: P2)

Tapping a download opens a view with the whole story: artwork, title, artist or
publisher, the source link, music or music video, whether it came from a single
link, a playlist or a channel, where it is now, whether lyrics landed and in what
language, why it failed if it did, and when it was created and when it reached
its final state — dates as `2026-11-21` and times in 24-hour form down to the
second.

**Why this priority**: It is where every fact this spec gathers becomes visible.
It depends on Story 2 for the record and Story 3 for the lyrics detail.

**Independent Test**: Open a finished download and confirm every listed fact is
present and correctly formatted, and open a failed one and confirm its reason and
both timestamps are present.

**Acceptance Scenarios**:

1. **Given** a completed download, **When** the user taps it, **Then** a detail
   view shows its artwork, title, publisher, link, mode, scope, state, lyrics
   status and language, and its creation and final-state timestamps.
2. **Given** any timestamp shown anywhere in this feature, **When** it is
   rendered, **Then** its date reads year-month-day as `2026-11-21` and its time
   reads 24-hour with seconds, as `14:32:07`.
3. **Given** a download that has not finished, **When** the user opens it,
   **Then** the final-state timestamp is absent rather than shown as blank or as
   a placeholder date.
4. **Given** a failed download, **When** the user opens it, **Then** the reason
   is shown in plain language and never includes a command line, a file path, or
   raw worker output.

---

### User Story 5 - Retry the one that failed (Priority: P2)

A download failed — the source was briefly unreachable, or the extractor had a
bad day. The user retries it from the row or from its detail view, and it goes
back into the queue as a fresh attempt.

**Why this priority**: Without it, the only recovery is to find the link again
and re-paste it, which is precisely the manual routine this feature exists to
remove.

**Independent Test**: Fail a download, retry it, and confirm a new attempt starts
against the same link and mode without the user re-entering anything.

**Acceptance Scenarios**:

1. **Given** a failed download, **When** the user retries it, **Then** a new
   attempt is queued for the same link and mode, and the row reflects the new
   attempt rather than duplicating into two rows.
2. **Given** a download that is queued, running, or completed, **When** the user
   looks at it, **Then** retry is not offered.
3. **Given** a retried download, **When** the user opens it, **Then** it is
   visible that this is a repeat attempt rather than the original.
4. **Given** a failed download belonging to another user, **When** a non-admin
   attempts to retry it, **Then** the request is refused.

---

### User Story 6 - A playlist or channel becomes its own items (Priority: P2)

The user pastes a playlist or a channel. Instead of one opaque row that either
works or does not, SynoDL works out what the link contains and then shows each
item as its own download, with its own title, artwork, progress and outcome. One
item failing does not take the rest with it, and the user can retry just that one.

**Why this priority**: It is the largest piece of new machinery here and the
biggest change to how bulk requests behave, but it depends on Stories 2, 3 and 5
being in place first to be worth anything.

**Independent Test**: Submit a playlist link and confirm it resolves into one
download per entry, each independently trackable and independently retryable.

**Acceptance Scenarios**:

1. **Given** a playlist link, **When** the user submits it, **Then** the request
   first reports that it is working out what the link contains, and then becomes
   one download per entry.
2. **Given** a channel link, **When** it is expanded, **Then** every full-length
   item the channel publishes becomes its own download, with no ceiling on how
   many.
3. **Given** an expanded playlist where one entry is private or removed, **When**
   the run proceeds, **Then** that entry fails on its own and every other entry
   still downloads.
4. **Given** an expansion that cannot be completed at all, **When** it fails,
   **Then** the original request is reported as failed with a plain reason, and
   no partial set of items is left behind in an unclear state.
5. **Given** an expanded set, **When** the user looks at the Tasks list, **Then**
   it appears as one row for the playlist or channel reporting how many of its
   items are saved, failed and remaining — not as one row per item.
6. **Given** that row, **When** the user opens it, **Then** every item is listed
   with its own state and progress, and each can be opened, retried and dismissed
   on its own.
7. **Given** a channel of several hundred items, **When** it is expanded, **Then**
   the user's other downloads remain visible in the Tasks list without scrolling
   past the channel's contents.

---

### User Story 7 - Four at a time, the rest wait their turn (Priority: P2)

The user queues up twenty songs, or expands a large channel. Four download at
once; the rest sit in a visible queue and start automatically as each running one
finishes, whether it succeeded or failed.

**Why this priority**: Without it, expanding a channel with no ceiling would
create hundreds of simultaneous requests. It is what makes Story 6 safe.

**Independent Test**: Queue more than four downloads and confirm exactly four run
at a time, the rest show as waiting, and each finish starts the next.

**Acceptance Scenarios**:

1. **Given** more downloads queued than the limit allows, **When** the user
   looks, **Then** exactly the limit are running and the rest are shown as
   *queued*, visibly distinct from one that is *scheduled* and starting.
2. **Given** a running download reaches any final state, **When** a waiting one
   exists, **Then** that one starts without the user doing anything.
3. **Given** downloads are queued and running, **When** the server restarts,
   **Then** the queue resumes rather than being lost, and no more than the limit
   run at once.
4. **Given** a waiting download, **When** the user dismisses it, **Then** it
   leaves the queue and never starts.
5. **Given** the operator has changed the limit, **When** downloads are queued,
   **Then** the new limit is what is enforced.
6. **Given** one user has expanded a channel of hundreds of items, **When**
   another user submits a single link, **Then** that link starts without waiting
   for the channel to finish.
7. **Given** a user has a channel expanding and then pastes a single link of
   their own, **When** a slot frees, **Then** their pasted link is admitted before
   the remaining channel items.

---

### User Story 8 - Nothing lands under Unknown Artist (Priority: P3)

Every saved item carries cover art and sensible artist and album information. If
the item itself does not say who the artist is, the channel name stands in; if it
does not say what the album is, the playlist or channel name stands in before any
generic fallback.

**Why this priority**: A wrong or missing tag is invisible in SynoDL and glaring
in the media server, which is where the user actually listens. It is a polish
pass over an existing recipe rather than new machinery.

**Independent Test**: Download an item with no artist or album metadata from a
named playlist and confirm the saved file is shelved under the channel and
playlist names, with embedded cover art, in both music and music-video modes.

**Acceptance Scenarios**:

1. **Given** an item that declares no artist, **When** it is saved, **Then** it
   is filed and tagged under the name of the channel that published it.
2. **Given** an item that declares no album and came from a named playlist or
   channel, **When** it is saved, **Then** that playlist or channel name is its
   album, in the folder and in the tag alike.
3. **Given** an item saved as music, **When** the media server reads it, **Then**
   it shows cover art without a separate artwork file being needed.
4. **Given** an item saved as a music video, **When** the media server reads it,
   **Then** it shows cover art.
5. **Given** an item that came from an expanded playlist, **When** it is saved,
   **Then** it carries the same playlist-derived album information as it would
   have if the playlist had been fetched as one request.

---

### Edge Cases

- **The same link, twice.** A link already queued or running for the same mode is
  refused as a duplicate, as it is today. A link with a *completed* record is a
  different question — see FR-020.
- **A very large channel.** With no ceiling on expansion and no bound on history,
  a channel of thousands of items becomes thousands of permanent records. The
  queue keeps the cluster safe; the list and the poll cost are what must not
  degrade.
- **Worker output is unavailable.** A worker that has not started, whose output
  cannot be read, or that produced nothing parseable must still yield a correct
  life state — the download simply shows no percentage.
- **A state that never resolves.** A *resolving* request whose expansion worker
  vanishes, or a *scheduled* download the orchestrator never starts, must reach a
  final state rather than sitting indefinitely (FR-013d).
- **Progress that goes backwards or exceeds completion.** A worker fetching a
  video's picture and sound as separate streams reports two runs of 0–100%. The
  displayed progress must not appear to restart or exceed completion.
- **A worker finishes but wrote nothing.** Reporting an unsuccessful download as
  completed remains the one outcome that is never acceptable (0012 FR-018).
- **Retry of a download whose record outlived its link.** The source may have
  been removed since. A retry that then fails is an ordinary failure, not an
  error in the retry.
- **Expansion of a link that turns out to contain exactly one item.** It becomes
  one ordinary download rather than a group of one.
- **Dismissing a group while its items are still running.** The records go at
  once, but a running worker is left to finish rather than stranded mid-write; its
  record is removed when it does.
- **Dismissing then re-submitting the same channel.** Because a dismissed record
  means the item is no longer held (FR-020a), every item is fetched again — the
  library's own already-downloaded check is what keeps that cheap rather than
  wasteful.
- **A group whose every item is already held.** Re-submitting a channel that has
  nothing new must say so plainly rather than presenting an empty group.
- **A group of one.** A playlist that contains a single item should not force the
  user through a group view to reach it (see the expansion edge case above).
- **The server restarts mid-expansion.** An expansion that was in flight must
  either complete or be reported as failed; it must not leave a request stuck
  forever in "working out what this contains".
- **Two users submit the same channel.** The second must not silently inherit or
  duplicate the first's items, and fair-share must not let the pair of them
  monopolise every slot.
- **A group that never fully finishes.** A group notification fires only once
  every item is final; a group with one item stuck must still reach that point
  (FR-013d) or it would never notify at all.
- **One user, nothing but bulk work.** Where a single user is the only one with
  anything queued, fair-share must not idle slots — the limit is a ceiling, not a
  reservation.
- **A source that names itself something hostile.** A playlist or channel whose
  title contains path separators, parent-directory references, or thousands of
  characters must not be able to place a file outside the mounted library or
  produce an unusable path (FR-038a, FR-038b).
- **The state volume fills.** A download that saved its files but whose record
  could not be written must not be reported as failed (FR-006b).
- **Clock and locale.** The date and time format is fixed by this spec and does
  not follow the viewer's device locale.

## Requirements *(mandatory)*

### Functional Requirements

**A durable record of every download**

- **FR-001**: System MUST keep a durable record of every YouTube download
  request, not only those that failed, so it remains visible after the
  orchestrator has swept the job that performed it.
- **FR-002**: A download record MUST carry its source link, its mode, its scope,
  the user who submitted it, its title, its publisher, and its artwork reference,
  where the source made those known.
- **FR-003**: A download record MUST carry the time it was created and, once it
  reaches a final state, the time it reached it.
- **FR-004**: System MUST continue to derive an in-flight download's state from
  the orchestrator and its worker rather than from a stored mirror, so a restart
  cannot desynchronise a running download.
- **FR-005**: A download record MUST be removable by the user who owns it, and
  removing it MUST NOT delete anything the download saved.
- **FR-005a**: Dismissing an expanded playlist or channel MUST remove the group
  and every one of its item records together — a group is one thing to the user
  and must behave as one when discarded.
- **FR-005b**: Dismissing a group MUST remove its queued items from the queue so
  they never start, MUST leave any already-running item to finish rather than
  stranding its worker, and MUST remove each running item's record once it
  finishes.
- **FR-005c**: Dismissal MUST NOT be refused because a download is queued or
  running; calling off work already started is the point of it.
- **FR-006**: Download history MUST be kept until its owner dismisses it. There
  is no age limit and no row cap: with no ceiling on expansion, a complete record
  is the point, and the user is the one who decides what to forget.
- **FR-006a**: Because history is unbounded, listing downloads MUST stay
  responsive at thousands of records — the list MUST NOT require loading the
  whole history to show the current page of it.
- **FR-006b**: Because history is unbounded and the state volume is finite,
  System MUST fail safely if that volume cannot be written: a download record that
  cannot be stored MUST NOT cause a download already saved to be reported as
  failed, and the operator MUST be able to tell that this happened.
- **FR-006c**: Extending the store MUST be reversible in the sense that a failed
  or partial schema change leaves the existing data intact and the application
  able to report the problem rather than starting against a half-changed store.
- **FR-006d**: When a user is deleted, their download records MUST survive as
  unattributed records rather than being erased — matching how NAS tasks and the
  existing failure records already outlive the account that created them — but
  their queued downloads MUST leave the queue and never start, and any worker
  already running MUST be left to finish rather than stranded (FR-005b). An
  unattributed record is visible to admins only, since FR-007 gives it no owner.

**Ownership and visibility**

- **FR-007**: A user MUST see only the YouTube downloads they submitted; an admin
  MUST see every user's, each attributed to its submitter.
- **FR-008**: Every action on a single download — viewing its detail, retrying
  it, dismissing it — MUST be refused for a user who does not own it and is not
  an admin, and MUST NOT disclose that download's existence or content.
- **FR-009**: The submitter's name MUST remain visible only to admins, as it
  already is for NAS tasks.
- **FR-009a**: An admin MAY view, retry and dismiss any user's download, stated
  here positively so the permission is a decision rather than an inference from
  FR-008's exception.
- **FR-009b**: Dismissing a group MUST be permitted only to its owner and to an
  admin, since it cancels queued work; an admin doing so MUST be subject to the
  same rules as the owner (FR-005b).
- **FR-009c**: The queue and its fair-share ordering MUST NOT reveal anything
  about another user's downloads to a non-admin. A user may see where their own
  downloads stand; they MUST NOT learn what else is queued or by whom.
- **FR-009d**: The artwork proxy MUST be reachable only by a signed-in user and
  MUST NOT be usable to fetch anything other than artwork for a download, so
  making downloads private does not leave their artwork publicly addressable.

**Progress and what the worker did**

- **FR-010**: System MUST report a running download's progress toward completion,
  derived from what the worker itself reports about its own work.
- **FR-011**: System MUST report, for a finished download, whether a lyrics or
  subtitle companion file was saved and in which language.
- **FR-012**: System MUST render progress that only advances, never appearing to
  restart or exceed completion, even where the underlying work happens in more
  than one pass.
- **FR-013**: Where progress cannot be determined, System MUST still report the
  download's correct life state and MUST NOT display an invented or stale
  percentage. Losing progress MUST NOT cause a download to be reported as failed.
- **FR-013e**: Reading a worker's output MUST be bounded in size, so a worker that
  produces far more output than expected cannot become a memory problem for the
  server.
- **FR-013f**: Reading a worker's output MUST be bounded in frequency and MUST NOT
  scale with the number of viewers watching. A download's progress is read because
  the download is running, not because someone is looking at it.
- **FR-013g**: Facts that outlive the worker — whether a lyrics file was written,
  in which language, and the final progress — MUST be captured into the durable
  record while the worker's output is still readable. The record is durable; the
  output it came from is not.
- **FR-013h**: A worker's output MUST be treated as untrusted external input
  wherever it is read, including the output of an expansion worker, which carries
  many titles and links at once.

**The states a download moves through**

- **FR-013a**: A download MUST be in exactly one of six states: *resolving*
  (working out what a playlist or channel link contains), *queued* (SynoDL is
  holding it behind the parallel limit), *scheduled* (admitted; the orchestrator
  is starting its worker), *downloading* (the worker is fetching, and progress
  applies), *completed*, or *failed*.
- **FR-013b**: *Queued* and *scheduled* MUST be distinguishable to the user, so
  "SynoDL is holding this behind others" is never confused with "this is starting
  now". Only *downloading* carries progress.
- **FR-013c**: Legal transitions are: *resolving* → *failed*, or *resolving* →
  the group's items each entering *queued*; *queued* → *scheduled* → *downloading*
  → *completed* or *failed*; and *failed* → *queued* on an explicit retry. A
  download MUST NOT leave a final state by any other route.
- **FR-013d**: *Completed* and *failed* are final. Every other state MUST be able
  to reach a final state without user intervention, so nothing can sit forever in
  *resolving*, *queued*, or *scheduled*.

**Playlists and channels become individual items**

- **FR-014**: A playlist or channel link MUST be resolved into the individual
  items it contains, and each item MUST become a download in its own right with
  its own title, artwork, progress, state and history.
- **FR-015**: System MUST expose a state meaning "working out what this link
  contains", distinct from waiting and from downloading, since the four states of
  spec 0012 cannot express it.
- **FR-016**: Resolving a link MUST happen outside the SynoDL server process, in
  the same short-lived-worker model as a download, and MUST be bounded in time.
- **FR-016a**: Every link that reaches a worker MUST pass the host allowlist,
  including one produced by expansion rather than typed by a user. Spec 0012
  validated only what a user submitted; expansion introduces links SynoDL derived
  from an external source, and those are not more trustworthy for having come from
  a link that was itself allowed.
- **FR-016b**: An expanded item MUST be identified by a stable identity derived
  from its link rather than by the link's text, so two spellings of the same item
  cannot defeat the already-held check of FR-020 or the duplicate check.
- **FR-017**: System MUST NOT impose a ceiling on how many items a playlist or
  channel expands into.
- **FR-018**: A single item failing MUST NOT prevent the remaining items of the
  same playlist or channel from downloading.
- **FR-019**: An expanded playlist or channel MUST remain a single row in the
  Tasks list, showing what it is and how its items are progressing as a whole —
  how many are saved, how many failed, how many remain.
- **FR-019a**: Opening that row MUST show the group's items, each as a download in
  its own right with its own artwork, title, state, progress, retry and detail.
- **FR-019b**: An expanded item MUST NOT appear as a top-level row in the Tasks
  list, so a large channel cannot crowd out a user's other downloads.
- **FR-019c**: An expanded item MUST make clear which playlist or channel it came
  from when viewed on its own.
- **FR-020**: Re-submitting a playlist or channel MUST NOT re-download items it
  already holds. "Already holds" means SynoDL has a completed record for that
  item in that mode: such items MUST be skipped at expansion, before any download
  is queued for them, so a re-run queues only what is genuinely new and creates no
  rows that would immediately do nothing.
- **FR-020a**: An item whose record the user has dismissed MUST be treated as not
  held, and so MUST be queued again on a re-run — dismissing forgets a download,
  and forgetting it means asking for it again is a real request.

**A bounded, durable queue**

- **FR-021**: System MUST run no more than a configured number of YouTube
  downloads at once, defaulting to four.
- **FR-022**: A request beyond that limit MUST wait in a queue that is visible to
  its owner, and MUST start automatically when a running download reaches any
  final state.
- **FR-022a**: The limit is instance-wide, not per user, so total load on the
  cluster and on the source does not grow with the number of users.
- **FR-022b**: Slots MUST be shared fairly between users: no user's queued
  downloads may be starved by another user's, however many that user has queued.
- **FR-022c**: Within one user's own queue, a directly submitted link MUST be
  admitted ahead of that user's items from an expanded playlist or channel — a
  pasted link is an immediate want, a bulk expansion is background work.
- **FR-023**: The queue MUST survive a restart of the SynoDL server, resuming
  rather than stranding or duplicating waiting work.
- **FR-024**: A *queued* download MUST leave the queue and never start when its
  owner dismisses it (FR-005b).
- **FR-025**: The parallel limit MUST be configurable by the operator.

**Notifications**

- **FR-025a**: YouTube downloads MUST honour each user's existing notification
  preferences — added, completed, failed — rather than introducing a separate set
  of switches.
- **FR-025b**: A directly submitted link MUST notify as one download.
- **FR-025c**: An expanded playlist or channel MUST notify at most once for the
  group as a whole, when every item has reached a final state, summarising how
  many were saved and how many failed. It MUST NOT notify per item.
- **FR-025d**: Notification MUST respect ownership: a user is notified only about
  downloads they can see (FR-007), under their existing notification scope.

**Retry**

- **FR-026**: Users MUST be able to retry a failed download without re-entering
  its link or re-choosing its mode.
- **FR-027**: System MUST NOT retry any download automatically; every retry is an
  explicit user action (0012 FR-021 stands).
- **FR-028**: Retry MUST be offered only for a download in a failed state.
- **FR-029**: A retried download MUST remain one download in the user's history,
  showing that it was attempted again rather than appearing twice.

**Detail view and presentation**

- **FR-030**: Users MUST be able to open a single download and see everything
  known about it: artwork, title, publisher, source link, mode, scope, state,
  progress, lyrics status and language, failure reason where applicable, and both
  timestamps.
- **FR-031**: Every date shown by this feature MUST render as year-month-day
  (`2026-11-21`) and every time as 24-hour with seconds (`14:32:07`), regardless
  of the viewer's device locale.
- **FR-032**: A failure reason shown to a user MUST remain plain language and
  MUST NOT contain a command line, a file path, a library path, or raw worker
  output. This applies to EVERY failure this feature can produce — a download's,
  an expansion's, a queue admission's, and a retry's — not only a download's.
- **FR-032a**: The values this feature newly handles that MUST stay out of logs,
  metrics and error payloads are: raw worker output, expansion results, saved file
  paths, library paths, source links, and playlist or channel names. Progress
  figures, state names and language codes are safe to expose.
- **FR-032b**: A failure of SynoDL's own machinery — the orchestrator being
  unreachable, or a worker's output being unreadable — MUST be reported to the
  user as distinct from the download itself having failed, so an infrastructure
  problem is never mistaken for a broken link.
- **FR-033**: An absent fact MUST be shown as absent rather than as an empty
  value or a placeholder — including a final-state timestamp on a download that
  has not finished.

**What gets saved**

- **FR-034**: Every saved item MUST carry embedded cover art, in music and
  music-video modes alike. Where the media server cannot read embedded art for a
  given format, an accompanying artwork file MUST be saved instead.
- **FR-035**: Where an item declares no artist, the publishing channel's name
  MUST be used as its artist, in the folder structure and in the saved tags
  alike.
- **FR-036**: Where an item declares no album, the playlist or channel name MUST
  be used as its album before any generic fallback.
- **FR-037**: An item downloaded as part of an expanded playlist or channel MUST
  receive the same playlist-derived artist and album information it would have
  received had the playlist been fetched as a single request.
- **FR-038**: Any value derived from a playlist or channel name and passed to a
  worker MUST reach it as a discrete argument, never assembled into a command
  string.
- **FR-038a**: A source-derived name is also used as a FOLDER name inside the
  mounted library. It MUST NOT be able to place a file outside that library,
  whatever the source called itself. Discrete-argument passing (FR-038) prevents a
  command being injected; it does nothing about a path, and both MUST be prevented.
- **FR-038b**: A source-derived value passed to a worker MUST be bounded in length
  and constrained in the characters it may contain, so a pathological title cannot
  produce an unusable path or an oversized argument list.
- **FR-038c**: The full set of user- or source-influenced values that reach a
  worker MUST be enumerated in the plan, so each can be checked against FR-038,
  FR-038a and FR-038b individually. As of this spec they are: the submitted link,
  and the playlist or channel name.

### Key Entities

- **Download**: One request for one item, from creation to final state. Carries
  the source link, mode, scope, submitter, what the source said it is (title,
  publisher, artwork), its current state and progress, its outcome and reason,
  what companion files it produced, both timestamps, and how many attempts it has
  had. This is the durable record; the running worker, when there is one, is not.
- **Download group**: A playlist or channel request and the items it resolved
  into. It is itself a row in the Tasks list, carrying the playlist's or channel's
  own title and artwork and an aggregate of its items' outcomes; it gives each
  expanded item its origin and its playlist-derived naming. Its items live behind
  it rather than beside it in the list.
- **Queue position**: Where a download sits relative to the parallel limit —
  *queued*, *scheduled*, or past both. Durable, so a restart resumes rather than
  restarts.
- **Worker report**: What a running worker says about its own progress and about
  the files it wrote. Read while the worker lives; never a mirror, and never the
  source of truth for whether a download succeeded.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can tell how far along a running download is within five
  seconds of looking at it, without opening anything.
- **SC-002**: 100% of downloads that reach a final state remain visible with
  their title, artwork, link and both timestamps after the worker that performed
  them no longer exists.
- **SC-003**: A user recovers from a failed download in one action, without
  finding or re-entering the original link.
- **SC-004**: Submitting a playlist of 20 items produces 20 independently
  trackable downloads, and one failing entry leaves the other 19 unaffected.
- **SC-004a**: Expanding a channel of any size adds exactly one row to the Tasks
  list, and the user can still see their other downloads without scrolling.
- **SC-005**: With any number of downloads queued, never more than the configured
  limit run at once, and the queue drains without user intervention.
- **SC-005a**: A user who submits a single link while another user's large channel
  is downloading waits for at most one running download to finish, not for the
  channel.
- **SC-005b**: Expanding a channel of any size produces at most one notification
  per user who asked to be notified.
- **SC-005c**: A user can call off an expanded channel of any size in one action,
  and no queued item of it starts afterwards.
- **SC-006**: A server restart with work in flight loses no queued download and
  misreports no running one.
- **SC-006a**: A history of several thousand downloads opens as quickly as a
  history of ten.
- **SC-007**: No user can see, open, retry or dismiss a download submitted by
  another user unless they are an admin, and no non-admin can learn from the queue
  what another user has queued.
- **SC-007a**: No source-controlled value can cause a file to be written outside
  the media library the download was aimed at.
- **SC-008**: 100% of saved items are shelved by the media server under a named
  artist and a named album with cover art, with no unknown-artist or
  unknown-album placeholders, in both modes.
- **SC-009**: Every date and time this feature displays follows the year-month-day
  and 24-hour-with-seconds format, on every device, regardless of device locale.

## Credential-Safety Impact

- **Widened: SynoDL may read a worker's own output.** Progress and companion-file
  reporting require reading what a worker prints about its own work. This widens
  the orchestrator permission granted in 0012, and it MUST widen it minimally: it
  remains confined to SynoDL's own namespace, remains scoped to workloads SynoDL
  itself created, and MUST NOT extend to reading secrets, executing inside pods,
  attaching to them, or reading anything SynoDL did not create. Reading a
  worker's output is not the same permission as controlling the worker.
- **Worker output is user data and stays out of logs.** A worker's output names
  the source link and the paths it wrote. Those are subject to the existing rule
  for task URIs: never logged, never in metrics, never in an error payload
  returned to a client. What reaches the user is a state, a percentage, a
  language, and a plain-language reason — never raw output.
- **New: a playlist or channel name reaches a worker.** Expansion introduces a
  second user-influenced value passed to something SynoDL executes. Like the
  submitted link, it MUST reach the worker as a discrete argument and MUST NEVER
  be assembled into a command string.
- **New: what is stored grows from failures to every request.** The store gains a
  record of every download, including successes: the link, mode, scope,
  submitter, title, publisher, artwork reference, outcome and timestamps. It
  lives in the existing single SQLite store under the one-store rule and
  introduces no second datastore. Submitted links are user-chosen public URLs
  rather than secrets, but remain user data — visible only to their owner and to
  admins, out of logs, and removable.
- **New: a durable queue is stored work, not mirrored state.** A waiting download
  has no worker, so recording it mirrors nothing. Once admitted, its live state
  is still read from the orchestrator, keeping the Principle III rule intact.
- **Ownership is now enforced, not merely displayed.** 0012 gated only the
  submitter's name behind admin; this spec makes ownership an access rule on
  every single-download action.
- **No new DSM API and no NAS credential.** As in 0012, this feature does not
  call Download Station and does not use the stored NAS connection. The server
  container still never mounts a media library.

**Constitution impact**: Principle III's "job state belongs to the orchestrator"
bullet and the least-privilege worker-orchestration rule both need amending to
admit (a) a durable record of requested and finished work including successes,
(b) a durable pre-admission queue, and (c) reading a worker's own output. The
amendment MUST land before or with the first implementation task that depends on
it — Principle I makes code without an approved constitutional basis a defect, so
it cannot follow the work. It is a MINOR bump (2.1.0 → 2.2.0): no principle is
removed or reversed, and Principle III's custody rules are widened rather than
relaxed. A checklist is required because this spec touches worker and cluster
credentials; it is `checklists/security.md`.

## Assumptions

- **The verified download recipe stands.** 0012's extraction, tagging, foldering
  and companion-file behaviour was verified end-to-end against real content. This
  spec changes it only where a requirement here demands it — album naming, cover
  art for video, per-item playlist naming, and machine-readable progress output.
- **The music-video cover art path is unverified.** Cover art for audio is known
  to work; the video path has not been confirmed against the pinned worker image
  and may silently skip embedding if a required tool is absent from it. FR-034 is
  written to be satisfied either by embedding or by an accompanying artwork file,
  and verifying which is required is part of the work.
- **A single admitter.** SynoDL runs as one replica, so exactly one thing decides
  what to admit from the queue. A multi-replica deployment is out of scope.
- **Fair-share is between users, not between groups.** Two channels expanded by
  the same user take that user's share between them in submission order; the spec
  does not promise to interleave them.
- **Expansion does not count against the parallel limit.** Working out what a link
  contains is short and cheap; the limit governs downloads.
- **A duplicate in-flight request is still refused, not queued** behind the first,
  as in 0012.
- **YouTube remains the only source.** The host allowlist is unchanged.
- **Local development and e2e still must not require a cluster.** The existing
  mock orchestrator must gain whatever is needed to exercise progress reporting,
  expansion, and the queue, so `make start` and e2e keep working on a laptop.
- **The date and time format is fixed by the product, not by the device.** It is
  a deliberate house style, not a locale-following behaviour.
- **History is the user's to prune, not the system's.** Nothing sweeps a finished
  download on age or on count; dismissing is the only way a record leaves. That
  choice follows directly from placing no ceiling on expansion — a complete record
  of a complete channel is the point.
- **Retry does not resume.** A retried download starts over rather than continuing
  a partial one; a partially-downloaded item is not a resumable transfer.

## Clarifications

### Session 2026-09-08

- Q: How should the server learn live progress and whether lyrics were written? →
  A: By reading the worker's own output. The alternative — having the worker call
  back into SynoDL — would require building and maintaining a wrapper image around
  the pinned upstream extractor, plus a per-job credential inside the pod and a
  network path back. Reading what the worker already prints needs neither, and the
  output can be a format SynoDL defines rather than text it scrapes. Carried into
  FR-010, FR-011 and the Credential-Safety Impact section as a minimal widening of
  the existing worker permission.
- Q: How far should a channel or playlist expand? → A: No ceiling. Every item the
  channel or playlist publishes becomes its own download. The risk this raises —
  hundreds of simultaneous requests — is answered by the parallel limit (FR-021)
  rather than by a cap, and the risk to the list is answered by FR-006a. Carried
  into FR-017.
- Q: After a playlist or channel expands, what appears in the Tasks list? → A: One
  row for the group, showing aggregate progress; the items live behind it and are
  reached by opening it. The Tasks list is a mixed list that also carries NAS
  downloads, and with no ceiling on expansion a flat layout would let one channel
  push everything else off the screen. Each item is still a full download in its
  own right once opened. Carried into FR-019–FR-019c, User Story 6, SC-004a, and
  Key Entities.
- Q: How are the four parallel slots allocated when several downloads are waiting?
  → A: One instance-wide limit of four, shared fairly between users, with a
  directly submitted link admitted ahead of the same user's bulk-expanded items.
  Strict first-in-first-out would let a single 500-item channel block every other
  user for hours, and a per-user limit would let total cluster load grow without
  bound. Fair-share is a small addition to a queue this spec requires anyway.
  Carried into FR-022a–FR-022c, User Story 7, SC-005a, Edge Cases and Assumptions.
- Q: Do YouTube downloads send push notifications, and at what granularity? → A:
  Yes, under each user's existing notification preferences from spec 1004 — a
  pasted link notifies as one download, and an expanded playlist or channel
  notifies once for the group when every item is final, summarising saved and
  failed counts. Introducing a separate set of switches would surprise a user who
  has already chosen to be notified, and notifying per item would send hundreds of
  pushes from one paste. Carried into FR-025a–FR-025d, SC-005b and Edge Cases.
- Q: What happens when a user dismisses an expanded playlist or channel? → A: The
  group and every item record go together; queued items leave the queue, running
  ones are left to finish and are then removed, and nothing already saved is
  touched. A group is one row to the user, so it must be discardable as one thing
  — and refusing dismissal until a 340-item channel finished would leave no way to
  call off a mistake. Carried into FR-005a–FR-005c, User Story 2, SC-005c and Edge
  Cases.
- Q: How long is download history kept, given that decision? → A: Until its owner
  dismisses it. No age limit, no row cap. A complete record of a complete channel
  is the point of having no ceiling; the consequence is that listing must stay
  responsive at thousands of rows. Carried into FR-006, FR-006a and SC-006a.
- Q: What is the full set of lifecycle states a download can be in? → A: Six —
  resolving, queued, scheduled, downloading, completed, failed. Queued and
  scheduled stay visibly separate so "SynoDL is holding this behind others" is not
  confused with "the worker is starting now", which are different waits with
  different expected durations. Carried into FR-013a–FR-013d, Key Entities, and
  User Story 7.
- Q: When a channel or playlist is re-submitted, what counts as already having an
  item? → A: SynoDL's own completed record for that item in that mode. Skipping
  happens at expansion, before anything is queued, so a re-run does not create
  rows that would immediately complete doing nothing. A dismissed record means the
  item is no longer held, so it is fetched again. Carried into FR-020 and FR-020a.
- Deferred with a documented default rather than asked (marker limit): whether
  resolving a link counts against the parallel limit (it does not — it is short
  and cheap), and whether retry resumes a partial download (it does not — it
  starts over). Both are recorded in Assumptions and revisitable in the plan.

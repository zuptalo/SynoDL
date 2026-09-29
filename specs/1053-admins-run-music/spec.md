# Feature Specification: Admins run the music library repair from Settings

**Feature Branch**: `feat/1053-admins-run-music`

**Created**: 2026-09-29

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Let administrators run the music library repair (spec 1052) from Settings instead of with kubectl: check the library, review what would change, apply it, undo it, with a history of runs. Only admins, only one run at a time, the same safety properties as the tool, no new server permissions, the server never reads the library."

## Overview

Spec 1052 shipped the music library repair as an **operator tool**: a person with
`kubectl` runs `scripts/music-repair.sh`, which creates a one-shot Job that plans,
applies or restores a repair, and then reads the plan off the share. It worked —
on the live library it removed 1,774 duplicates, filed 4,000 songs, wrote 88
playlists and failed nothing — but only someone with cluster access could run it,
and the result was a markdown file to go and find on the NAS.

This spec puts that same tool behind an **admin-only section in Settings**. An
admin taps *Check library*, watches it run, reads a plain summary of what it would
change, and — if they agree — taps *Apply*. *Undo* reverses the last applied
repair, and a history lists what ran, who started it and how it ended.

Nothing about the repair itself changes: the same dry run that changes nothing, the
same `.trash` instead of deleting, the same refusal to overwrite, the same skipped
stale steps, the same resumable and reversible behaviour. What changes is who can
start it and where they see the answer.

The hard part is what the server is allowed to know. The server never mounts the
music library (constitution, Domain Constraints), so it cannot open a plan file. It
learns about a run only from the Job it launched and from what that worker chooses
to report, read in a bounded, parsed, never-logged way (constitution, Principle III).
Everything the admin sees is therefore a summary the worker reports — the complete
plan stays on the share, where the tool has always put it.

## Clarifications

### Session 2026-09-29

- Q: How hard should *Apply* be to confirm? → A: The confirmation shows what will change and the *Apply* button stays disabled until the admin ticks "I have taken a snapshot of the music share". A snapshot is the one protection that survives both a bad plan and a lost volume, and the app cannot verify it exists, so the admin acknowledges it. No typed word.
- Q: How long is a check good for before it must be re-run? → A: 24 hours. That leaves a working day to review and decide; the tool's per-file staleness check still protects anything that changed; and a shorter window would force hours-long re-checks on a large library because of the metadata sources' rate limit.
- Q: How much of "left alone" does the screen show? → A: Counts by reason plus up to 20 example library-relative paths in total, with the overall total. The full list stays on the share; a bounded, fixed shape keeps the worker's report safe to parse and stops a large slice of the library reaching the browser.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Check the library and read what would change (Priority: P1)

An admin opens Settings, taps *Check library*, sees it working (which stage, how far
along), and when it finishes reads a plain summary: how many duplicates would be set
aside, how many files moved and retagged, how many playlists written, how many
songs matched and how many would stay in Singles, whether the volume has the space,
and what was left alone and why. Nothing on the share has changed.

**Why this priority**: it is the whole value of reviewing before acting, it is
useful on its own as an audit, and everything else builds on it. It is also the
safe half: it can be built, shipped and used with no way to change the library.

**Independent Test**: as an admin, start a check against the mock cluster, watch
progress advance, and read a result whose numbers equal what the worker reported.
Confirm the mock library is byte-identical afterwards.

**Acceptance Scenarios**:

1. **Given** an admin and a configured music library, **When** they tap *Check
   library*, **Then** a check starts, its stage and progress are shown, and it
   survives closing and reopening the screen.
2. **Given** a finished check, **When** the admin opens it, **Then** they see
   duplicates, moves, retags, playlists, conflicts, matched / no-match / not-looked-up
   counts, songs bound for Singles, the space needed against the space free, and the
   files left alone with their reasons.
3. **Given** a finished check, **Then** they are told where the complete plan is on
   the share, because the screen shows a summary and not every line.
4. **Given** a check that is running, **When** an admin (the same or another) tries to
   start another repair run, **Then** it is refused and they are told who started
   the running one and when.

---

### User Story 2 - Apply a plan the admin has just reviewed (Priority: P1)

Below a finished check the admin sees *Apply this plan*. It asks for an explicit
confirmation that says what will happen — duplicates go to `.trash`, files are
moved and retagged, playlists are written — that nothing is deleted, that it can be
undone, and that a snapshot of the share beforehand is advisable. On confirming, the
repair runs and shows progress, then reports how it ended.

**Why this priority**: it is the point of the feature, and the riskiest thing in
it, so its safety rules are as important as its existence.

**Independent Test**: against the mock cluster, run a check, apply it, watch
progress and read the outcome; try to apply with no check, an old check, a check
that failed, and a plan that was already applied — each is refused.

**Acceptance Scenarios**:

1. **Given** a check that finished successfully and is recent, **When** the admin
   confirms *Apply*, **Then** exactly that plan is applied and progress is shown.
2. **Given** no finished check, a failed check, a check older than the freshness
   window, or a plan already applied, **Then** *Apply* is not offered and a direct
   request is refused.
3. **Given** the confirmation dialog, **Then** it states what will change, that
   nothing is deleted and that it can be undone, and *Apply* stays disabled until
   the admin ticks "I have taken a snapshot of the music share"; nothing is started
   by a stray tap, and un-ticking disables it again.
4. **Given** an apply that was stopped part-way (a deadline, a restart, a node
   drain), **Then** it is shown as *did not finish* with what it had done, and the
   admin can continue it, which resumes rather than starts over.
5. **Given** steps the tool skipped because the library changed since the plan was
   made, **Then** the outcome lists them as skipped, with the reason.

---

### User Story 3 - Undo the last applied repair (Priority: P2)

An admin who does not like the result taps *Undo* on the last applied repair and,
after a confirmation, the files and tags it changed are put back.

**Why this priority**: it is what makes applying a reasonable thing to try, but it
is used rarely and depends on Story 2.

**Independent Test**: apply on the mock cluster, then undo; the outcome reports what
was restored and what could not be (an original location that is now occupied).

**Acceptance Scenarios**:

1. **Given** an applied repair, **When** the admin confirms *Undo*, **Then** the
   tool's restore runs and the outcome says how many things were put back and what
   was skipped and why.
2. **Given** a repair that has already been undone, or one that never applied,
   **Then** *Undo* is not offered.
3. **Given** a run in progress, **Then** *Undo* is not offered until it ends.

---

### User Story 4 - See what has run (Priority: P2)

An admin opens the history and sees past checks, applies and undos: what kind,
who started it, when, how long it took, and how it ended, with the summary of each.

**Why this priority**: it answers "did anyone already run this, and what happened?"
and lets an admin find the plan they are about to act on. It adds no capability.

**Independent Test**: run a few operations as two different admins; the history
lists each with the right person, time and outcome, newest first, and survives a
server restart.

**Acceptance Scenarios**:

1. **Given** past runs, **Then** each shows its kind, who started it, when, its state
   and its summary.
2. **Given** the server restarted while a run was in progress, **Then** the history
   is intact and the run's real state is shown, not a stale one.
3. **Given** a very long history, **Then** only the recent part is kept and shown;
   nothing about the library depends on the older rows.

---

### User Story 5 - The feature is honest about when it cannot run (Priority: P3)

Where the repair cannot run — no cluster, no music library configured, or a person
who is not an admin — the section says so plainly (or is absent for a non-admin) and
the rest of the app behaves exactly as it did.

**Why this priority**: it is what keeps the rest of the app safe from this feature,
but it is exercised only where the feature is not.

**Independent Test**: run the app outside a cluster and with no music library set;
open Settings as an admin and as a normal user.

**Acceptance Scenarios**:

1. **Given** a normal user, **Then** the section is not shown and every request for it
   is refused.
2. **Given** no cluster access or no music library configured, **Then** an admin sees
   why the repair is unavailable and how to enable it, and nothing else in the app
   changes.

---

### Edge Cases

- Two admins tap *Check* or *Apply* at once: exactly one run starts; the other is
  told who is running and since when.
- The server restarts mid-run: the run continues (it is a Job), and on return the
  screen shows its real state and progress.
- A run's Job is deleted or vanishes with no result: it is shown as *did not finish*,
  never as completed.
- The worker's report is missing, oversized or malformed: the run is shown as
  finished with *no summary available*, and nothing raw is returned or logged.
- A check is made, then the library changes (a download lands): the plan's steps for
  the changed files are skipped at apply and reported, exactly as with the tool.
- A plan made from the command line exists on the share: this feature does not know
  about it, cannot apply it, and says so if asked — the server never reads the
  share. It remains usable from the command line.
- The admin who started a run is deleted: the history keeps the run, showing that its
  starter no longer exists.
- Apply is confirmed, then the server cannot create the Job: it is refused with a
  clear reason and nothing was started or recorded as running.
- A stale lock from a crashed run exists on the share: the run refuses and the
  screen relays the reason; it is never taken over silently (as with the tool).
- The check runs for hours (thousands of songs at a source's rate limit): progress
  keeps updating and the screen stays usable; closing it does not stop the run.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Only administrators MUST be able to see or use any part of this
  feature; every request for it MUST be refused for everyone else, whatever the
  client sends.
- **FR-002**: The feature MUST be unavailable — and say why — when SynoDL is not
  running against a cluster or no music library is configured, and in that case the
  rest of the application MUST be unaffected.
- **FR-003**: An admin MUST be able to start a **check** (the tool's plan), which
  changes nothing in the library except the tool's own working folder.
- **FR-004**: While a run is in progress the screen MUST show its kind, its stage and
  its progress, and MUST keep doing so after the screen is closed and reopened, by
  any admin.
- **FR-005**: A finished check MUST show, as plain numbers: duplicates set aside,
  files moved, files retagged, playlists written, conflicts, songs matched, songs
  with no confident match, songs not looked up, songs bound for Singles, space
  needed against space free, and the files left alone as counts by reason plus at
  most 20 example library-relative paths in total, with the overall total; the
  rest is in the complete plan on the share.
- **FR-006**: A finished check MUST say where the complete plan is on the music
  share, because the screen shows a summary.
- **FR-007**: *Apply* MUST be possible only for a plan that (a) was made by this
  feature, (b) finished successfully, (c) has not yet been applied, and (d) is
  younger than 24 hours, counted from when its check finished. Otherwise it MUST NOT be offered and a direct
  request MUST be refused with the reason.
- **FR-008**: *Apply* MUST require an explicit confirmation stating what will
  change, that nothing is deleted and that it can be undone, and MUST NOT be
  possible until the admin has acknowledged, by ticking a box in that confirmation,
  that they have taken a snapshot of the music share. The acknowledgement is per
  request, never remembered, and a request that does not carry it MUST be refused
  by the server, not only hidden by the screen.
- **FR-009**: *Apply* MUST carry out exactly the plan the admin reviewed, identified
  by that plan's id, never "whatever is latest".
- **FR-010**: An admin MUST be able to continue an apply that did not finish, which
  MUST resume it, not start a new one.
- **FR-011**: An admin MUST be able to **undo** the last applied repair after an
  explicit confirmation. *Undo* MUST NOT be offered for a repair that was never
  applied, was already undone, or while another run is in progress.
- **FR-012**: At most one run (check, apply or undo) MUST be in progress at any
  time; a second request MUST be refused, saying who started the running one and
  when. This MUST hold when two requests arrive at the same moment.
- **FR-013**: The history MUST list past runs with kind, who started them, when they
  started and finished, their state and their summary, newest first, and MUST
  survive a server restart. Only a bounded recent part need be kept.
- **FR-014**: A run's state MUST be derived from the Job that carries it, not from a
  stored mirror; a run whose Job has vanished or failed without a result MUST be
  shown as *did not finish*, never as completed. Only the request and its finished
  outcome are stored.
- **FR-015**: The server MUST NOT mount the music library or read anything from it.
  It MUST learn a run's progress and result only from what the worker reports.
- **FR-016**: Anything read from the worker's output MUST be bounded in size, parsed
  into a fixed known shape (anything else discarded), MUST NOT be logged, and MUST
  NOT be returned to a client raw. A missing, oversized or malformed report MUST
  degrade to "finished, no summary available".
- **FR-017**: The worker Job MUST be created by the server using the permissions it
  already has, with no new permission, no secret, no service-account token in the
  worker, and exactly one library mounted. The worker image MUST stay the pinned
  one.
- **FR-018**: The code the worker runs MUST be the same version as the server that
  launched it, so an upgrade of one cannot leave the other behind.
- **FR-019**: The safety properties of spec 1052 MUST hold unchanged: a check changes
  nothing; nothing is deleted (removed files go to `.trash`); nothing is overwritten;
  stale steps are skipped and reported; a run is resumable; an apply is reversible;
  only the tool's own allowlisted hosts are contacted.
- **FR-020**: The operator tool (`scripts/music-repair.sh` and its package) MUST keep
  working unchanged, and a run started from the command line and one started from
  Settings MUST NOT run at the same time (the tool's own lock still decides).
- **FR-021**: The interface MUST be built from stock Ionic components in the existing
  Settings pattern (an admin entry that opens a modal), MUST work at phone width, and
  MUST work in both themes and right-to-left.
- **FR-022**: Nothing about a run — no path, tag, title, URL or worker output beyond
  the fixed summary — MAY appear in logs or error responses.

### Key Entities *(include if feature involves data)*

- **Repair run**: one request to check, apply or undo. Has a kind, who started it,
  when, the plan it belongs to, its state (running, finished, did not finish), and
  the summary of its result.
- **Plan (as known to the server)**: the id and the summary a finished check
  reported — not its contents. It has an age, and whether it has been applied or
  undone.
- **Summary**: the fixed set of numbers and bounded "left alone" reasons a worker
  reports for a run; the only view of a run the server ever holds.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An admin can check, review, apply and undo a repair using only the
  Settings screen, with no command line, in a test against the mock cluster.
- **SC-002**: A person who is not an admin cannot see the section and every request
  for it is refused — verified for each operation.
- **SC-003**: When two requests to start a run arrive at once, exactly one run
  starts, every time.
- **SC-004**: Closing the screen, or restarting the server, during a run never
  changes the run's outcome, and the screen shows its true state on return.
- **SC-005**: The server container mounts no library and opens no file on it, and
  the cluster permissions granted to the server are identical before and after this
  feature.
- **SC-006**: A check leaves the library unchanged, verified by comparing it before
  and after.
- **SC-007**: With the feature unavailable (no cluster, or no music library), every
  other screen and endpoint behaves exactly as before.
- **SC-008**: A finished check's numbers on the screen equal the numbers the worker
  reported, for every field.
- **SC-009**: The interface is usable at 360 px width with no horizontal scrolling.

## Credential-Safety Impact

- **Stored and how it is protected**: a bounded history of runs in the existing
  single SQLite store — kind, plan id, who started it, timestamps, state and the
  fixed summary numbers. No path, tag, URL, credential or worker output beyond the
  summary. It sits on the one state volume like the download records.
- **Crosses to the NAS**: nothing goes to the DSM API. The worker writes to the
  library through the mount it is given; the server never mounts the library.
- **Worker orchestration**: the server creates the Job with the namespaced
  permissions it already has. No new verb, no secret, no exec or attach, and the
  worker has no service-account token. Reading the worker's output is the narrower
  permission constitution v2.2.0 already allows, used only for the fixed summary.
- **What could appear in logs or errors**: run kind, state and counts only. Worker
  output is never logged and never returned raw; a malformed report degrades to "no
  summary available".
- **Outbound**: unchanged — the tool's own four public services, from the worker,
  over HTTPS. The server itself contacts nothing new.
- **Why safe**: an admin already holds the power to change the library through the
  NAS itself; this feature adds no capability an admin lacked, moves none to a
  non-admin, and keeps the tool's own reversibility.

## Assumptions

- The music library is the one already configured for downloads; the repair runs
  against that library only.
- A plan made from the command line is not visible here, because the server cannot
  read the share; it can still be applied from the command line.
- The freshness window for applying a plan is 24 hours (clarified). The tool's
  staleness check still protects any step whose file changed within it.
- The history keeps the most recent 50 runs.
- Notifying admins when a long run finishes (push) is out of scope for this spec.
- Editing a plan before applying it, and changing what new downloads write, are out
  of scope.
- The tool's complete plan and results remain on the share in the tool's own working
  folder; the screen shows a summary and says where the rest is.

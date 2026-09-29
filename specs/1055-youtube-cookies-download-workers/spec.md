# Feature Specification: YouTube sign-in for download workers

**Feature Branch**: `feat/1055-youtube-cookies-download-workers`

**Created**: 2026-09-29

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Add the same YouTube cookie setting SynoDL's sibling downloader has: an admin pastes the browser's Cookie header (or uploads a cookies file) once in Settings, and SynoDL's YouTube download workers use that sign-in so YouTube stops refusing them with 'Sign in to confirm you're not a bot'."

## Overview

On 2026-09-29 every YouTube download SynoDL attempted was refused: *"Sign in to
confirm you're not a bot."* The same links played fine in a browser on the same
connection. Nothing was wrong with the links, the worker image (the newest
extractor build was refused too) or the network address. YouTube simply treats an
anonymous, automated client as suspicious, and a signed-in browser session as
trustworthy. As soon as the downloader was given a signed-in browser session's
cookies, the same video resolved.

SynoDL's workers have no way to be given one. Today a refused download is retried
a few times, then reported as failed, and the operator's only remedy is to wait
and hope the refusal lifts.

This spec lets an **administrator give SynoDL a YouTube sign-in** — by pasting the
`Cookie` header they copy from the browser's developer tools, or by uploading a
cookies file — and makes SynoDL's YouTube download workers use it. The admin can
see that a sign-in is present and when it was added, replace it, and remove it.

A sign-in is a **live credential for a Google account**. That is the heart of this
spec: it must be stored encrypted, never shown again, never logged, never sent
anywhere but YouTube, and it must reach a short-lived worker without becoming
readable from the objects that describe the worker.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Give SynoDL a YouTube sign-in and downloads get through (Priority: P1)

An admin who sees YouTube downloads failing with the bot-check opens Settings, finds
*YouTube sign-in*, pastes the `Cookie` header from their browser (guidance on where
to find it is right there), and saves. From then on, YouTube downloads and the
playlist/channel expansions that precede them present that sign-in, and no longer
fail with the bot-check.

**Why this priority**: it is the whole feature, and without it every YouTube
download can fail for days.

**Independent Test**: against the mock stack, save a sign-in, start a YouTube
download, and assert the worker was given the sign-in for that download (and was
not given one before it was saved).

**Acceptance Scenarios**:

1. **Given** no sign-in saved, **When** a YouTube download runs, **Then** it behaves
   exactly as today.
2. **Given** a saved sign-in, **When** a YouTube download runs, **Then** its worker
   presents the sign-in to YouTube.
3. **Given** a saved sign-in, **When** a playlist or channel is expanded into items,
   **Then** the expansion also uses it (a playlist can be refused too).
4. **Given** a saved sign-in, **When** the admin removes it, **Then** later
   downloads run anonymously again and nothing of the sign-in remains stored.

---

### User Story 2 - The sign-in is protected like the credential it is (Priority: P1)

The admin can trust that pasting a Google session into SynoDL does not spread it.
It is stored encrypted at rest, it is never displayed or returned after saving, it
never appears in logs, errors, notifications or download history, and it is never
sent to any host other than YouTube's own.

**Why this priority**: an unprotected copy of a signed-in Google session is worse
than no feature at all. It is a Principle III requirement, not a nicety.

**Independent Test**: save a sign-in whose values are recognisable markers; then
read every API response, the server log, the stored database bytes, and the
description of the worker Job, and assert no marker appears in any of them.

**Acceptance Scenarios**:

1. **Given** a saved sign-in, **Then** no API response after saving contains any
   cookie value — only whether one exists, how many cookies, which login cookies were
   found, and when it was added.
2. **Given** a saved sign-in, **Then** the stored form is encrypted under the
   instance's secret key and is not readable from the database file.
3. **Given** a download running with the sign-in, **Then** none of the worker's Job
   description, arguments, annotations, labels or the server's logs contain a cookie
   value.
4. **Given** a non-admin user, **Then** they can neither read, save nor remove the
   sign-in.
5. **Given** a download from any site other than YouTube, **Then** its worker is not
   given the sign-in.

---

### User Story 3 - Saving is forgiving, and the admin is told what was saved (Priority: P2)

The admin does not have to reformat anything. They can paste the browser's `Cookie`
header exactly as copied (with or without the `Cookie:` prefix, with stray spaces or
newlines), or paste or upload a standard cookies file; SynoDL works out which it is.
On saving it says how many cookies it found and which of the login cookies are
present, and it warns — without refusing — when the login cookies are missing,
because that usually means the paste came from a request that was not signed in.

**Why this priority**: a wrong or empty paste looks exactly like a working one until
a download fails hours later. (An empty export cost real time on 2026-09-29.)

**Independent Test**: save each accepted shape and assert the same cookies result;
save junk, an empty paste, and a header with no login cookies and assert the clear
messages.

**Acceptance Scenarios**:

1. **Given** a pasted header with or without a `Cookie:` prefix, surrounding
   whitespace or line breaks, **Then** it is accepted and the cookie count is shown.
2. **Given** a cookies file in the standard tab-separated format, pasted or
   uploaded, **Then** it is accepted as it is.
3. **Given** text with too few cookies to be a session, or nothing recognisable,
   **Then** it is not saved and the reason is shown, without repeating the text.
4. **Given** cookies without any login cookie, **Then** it is saved but the admin is
   warned it is probably not signed in.

---

### User Story 4 - The admin can tell when the sign-in has stopped working (Priority: P2)

A saved sign-in does not last forever: Google can end the session, or the admin can
sign out of that browser. When YouTube refuses a download even with the sign-in
present, the admin should see that as a *sign-in* problem — "YouTube is refusing the
saved sign-in; replace it" — rather than a generic failure, and Settings should show
that the last attempts using it were refused.

**Why this priority**: without it a dead sign-in silently looks like the original
problem, and the admin cannot tell whether to wait or to re-paste.

**Independent Test**: with the mock source refusing, run a download with a sign-in
saved and assert the failure is reported as a sign-in problem and Settings shows the
refusal; run one with none saved and assert it stays the ordinary refusal.

**Acceptance Scenarios**:

1. **Given** a saved sign-in and a worker refused with the bot-check, **Then** the
   download's failure says the saved sign-in appears to have stopped working.
2. **Given** that, **Then** Settings shows the sign-in as last refused, with when.
3. **Given** a later download succeeds with it, **Then** Settings no longer shows it
   as refused.
4. **Given** no saved sign-in and a refusal, **Then** the failure is reported as
   today, with the added hint that saving a YouTube sign-in can fix it.

---

### User Story 5 - Everything else behaves as it does today (Priority: P3)

Without a saved sign-in nothing changes, in any mode, for any source. With one,
non-YouTube downloads and the music-video library are unaffected apart from YouTube
video downloads also presenting it.

**Independent Test**: run the existing download suites with and without a sign-in
saved and assert non-YouTube outputs are identical.

**Acceptance Scenarios**:

1. **Given** no sign-in, **Then** every existing download test passes unchanged.
2. **Given** a saved sign-in, **When** a non-YouTube link is downloaded, **Then** its
   worker is created exactly as before.
3. **Given** the stateless build (no secret key), **Then** there is no sign-in
   setting and nothing else is affected.

---

### Edge Cases

- **The session rotates.** YouTube may replace some cookies while a worker uses them.
  The worker cannot send cookies back to the server (the server learns from a worker
  only bounded, parsed, non-credential output), so a rotated session is not
  captured. If the saved session dies as a result, the admin sees the sign-in
  problem (User Story 4) and pastes a fresh one.
- **Many workers at once.** Several downloads run in parallel with the same saved
  sign-in; they must not interfere with each other or with the stored copy.
- **A worker outlives a cleared sign-in.** Removing the sign-in stops new workers
  using it; workers already running finish with what they were given, and nothing of
  it remains after they end.
- **A worker is killed or evicted.** No copy of the sign-in may linger past the
  worker's own life, whatever way the worker ends.
- **The paste is enormous or hostile.** It is size-bounded, parsed to cookies for
  YouTube's domains only, and anything else in it is dropped.
- **An operator upgrades from a build without the feature.** The setting is simply
  absent until used.
- **The secret key changes.** A stored sign-in that can no longer be decrypted is
  treated as absent and the admin is told to re-enter it, not shown an error trace.
- **Video downloads.** A YouTube link downloaded in music-video mode presents the
  sign-in the same way as a music download.
- **Playlists that mix in age-restricted or private items.** A saved sign-in may let
  more items through than before; that is expected and not an error.

## Requirements *(mandatory)*

### Functional Requirements

**Setting**

- **FR-001**: Settings MUST offer administrators a *YouTube sign-in* section to save,
  replace and remove a sign-in; other users MUST NOT see or use it.
- **FR-002**: The admin MUST be able to paste the browser's `Cookie` header, a
  cookies file's text, or upload a cookies file, and SynoDL MUST accept any of them
  (with or without the `Cookie:` prefix, tolerant of whitespace and line breaks).
- **FR-003**: The section MUST say where to find the header and that it is a live
  login and sensitive.
- **FR-004**: On saving, SynoDL MUST show how many cookies it found and which of the
  login cookies are present, and MUST warn (not refuse) when none are.
- **FR-005**: SynoDL MUST refuse, with a reason that does not repeat the pasted text,
  a paste with too few cookies to be a session or nothing recognisable, and MUST
  bound the size accepted.
- **FR-006**: Only cookies for YouTube's own domains MUST be kept; anything else in
  a pasted file is dropped.
- **FR-007**: The section MUST show, without any cookie value: whether a sign-in is
  saved, its cookie count, which login cookies it has, when it was saved, and
  whether the latest use was refused.
- **FR-008**: The feature MUST be unavailable in the stateless build, and the rest of
  Settings MUST be unaffected there.

**Use**

- **FR-009**: When a sign-in is saved, every YouTube download worker and every
  playlist/channel expansion worker MUST present it to YouTube.
- **FR-010**: A worker for any other site MUST NOT be given the sign-in.
- **FR-011**: When none is saved, workers MUST be created exactly as today.
- **FR-012**: Removing the sign-in MUST stop new workers using it and MUST leave
  nothing of it stored.

**Protection (Principle III)**

- **FR-013**: The saved sign-in MUST be stored encrypted at rest under the instance's
  secret key, in the single store, and MUST NOT be a second datastore.
- **FR-014**: No API MUST return cookie values after saving; responses carry only the
  metadata in FR-007.
- **FR-015**: Cookie values MUST NOT appear in logs, error messages, notifications,
  download history, Job arguments, annotations or labels, or the worker's reported
  output.
- **FR-016**: The sign-in MUST reach a worker without being readable from the
  worker's Job/pod description by anyone who can only read those objects, and no
  copy MUST outlive the worker, however it ends.
- **FR-017**: Delivering the sign-in MUST NOT widen the server's cluster permissions
  beyond what is explicitly justified and minimal, MUST NOT add a cluster-wide role,
  and MUST keep a worker mounting exactly one media library.
- **FR-018**: The worker image MUST stay pinned, and everything a user or a source
  supplies MUST still be host-allowlisted, passed as a discrete argument and
  sanitised into safe path segments.

**Refusals**

- **FR-019**: A refusal by YouTube while a sign-in is saved MUST be reported as a
  sign-in problem with a clear next step, and MUST be distinguishable from an
  ordinary refusal.
- **FR-020**: A refusal with no sign-in saved MUST be reported as today, plus a hint
  that saving a sign-in can resolve it.
- **FR-021**: The automatic re-queue of refused downloads MUST keep working as today
  and MUST NOT retry faster because a sign-in is saved.

### Key Entities *(include if feature involves data)*

- **YouTube sign-in**: one instance-wide set of cookies for YouTube's domains, stored
  encrypted, with non-secret metadata (cookie count, which login cookies exist, when
  saved, by whom, last refusal). At most one exists.
- **Sign-in status**: what Settings shows — present or not, count, login cookies
  found, saved-at, last-refused-at. Never any value.
- **Worker credential grant**: the short-lived way one worker is given the sign-in
  for its own lifetime and no longer.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With a valid sign-in saved, a YouTube download that fails with the
  bot-check without one succeeds, on the real cluster.
- **SC-002**: Across every API response, server log line, notification, download
  history entry, stored database byte and Job/pod description in the test suite,
  **zero** cookie values appear.
- **SC-003**: **Zero** non-YouTube workers are given the sign-in, across every
  supported source.
- **SC-004**: With no sign-in saved, **100%** of existing download tests pass
  unchanged.
- **SC-005**: Pasting the browser header takes the admin **one paste and one tap**,
  with no reformatting, and saves in under 5 seconds.
- **SC-006**: A refusal with a saved sign-in is reported as a sign-in problem in
  **100%** of test cases, and never as an ordinary failure.
- **SC-007**: After a worker ends by any route (success, failure, deadline,
  eviction), **zero** copies of the sign-in remain in the cluster beyond the
  server's own encrypted record.

## Credential-Safety Impact

This feature stores, moves and uses a live Google session, so it is the most
sensitive thing SynoDL has held. It amends nothing in the constitution, but it is
where the rules are tested hardest.

- **What is stored / protected**: one encrypted sign-in in the single store under
  the instance secret key, plus non-secret metadata. It is not a second datastore.
  It is never returned by the API after saving.
- **What crosses to third parties**: the sign-in is presented to YouTube's own
  hosts, by a worker, over HTTPS — and to no other host. No worker for another site
  is given it.
- **What crosses to the worker**: the sign-in is handed to one worker for that
  worker's lifetime. How it is delivered without becoming readable from the Job or
  pod objects, and without widening the server's permissions unjustifiably, is the
  central design question for planning (FR-016, FR-017).
- **Logs and errors**: cookie values are never logged, put in error text, echoed in
  a validation message, stored in download history or notifications, or reported in
  the worker's output.
- **Why the boundary holds**: admin-only; encrypted at rest; metadata-only reads; a
  discrete, bounded, YouTube-only delivery to a short-lived worker; the worker image
  pinned; permissions no wider than justified; and nothing left behind when the
  worker ends.

## Assumptions

- The sign-in is **instance-wide** and **admin-managed**; per-user sign-ins are out
  of scope.
- A refreshed session cannot be captured back from a worker (the server accepts only
  bounded, non-credential output), so a session that YouTube rotates or ends is
  replaced by the admin.
- The cookie format YouTube's extractor reads is the standard tab-separated cookies
  file; converting a pasted header to it is part of saving.
- The music-video library's downloads present the same sign-in when they are from
  YouTube.
- Other sources with their own login (the catalog sources) are unaffected and out of
  scope.
- Editing which cookies are used, per-download choice of sign-in, and refreshing the
  session automatically are out of scope.
- Delivery to the worker, and how much of the existing worker Job model is reused, are
  planning decisions bound by FR-016 to FR-018.

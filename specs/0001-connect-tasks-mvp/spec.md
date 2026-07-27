# Feature Specification: Connect to Download Station, view tasks, and add downloads

**Feature Branch**: `claude/synology-nas-pwa-client-s8x3m0` *(session-assigned; convention for future specs is `feat/0001-connect-tasks-mvp`)*

**Created**: 2026-07-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "A self-hostable PWA client for Synology NAS Download Station, matching the ease of use of the paid iOS client (screenshots provided): log in to the NAS, see and control the download task list with filters and sorting, and add new downloads by URL, clipboard, or .torrent file with a destination folder picker — because Synology's own web interface is not mobile friendly."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Connect to my NAS (Priority: P1)

As a NAS owner, I open SynoDL on my phone, sign in with my DSM account (with a
2-step verification code when my account requires one), and stay signed in
across app restarts until I log out or the NAS ends my session.

**Why this priority**: Nothing else works without a session; it is the front
door of the app.

**Independent Test**: With only the login screen implemented, a user can sign
in against the (mock) NAS, see an authenticated landing page, restart the app
without re-authenticating, and log out from Settings.

**Acceptance Scenarios**:

1. **Given** a signed-out visitor, **When** they open any URL in the app,
   **Then** they land on the login screen showing the configured NAS host.
2. **Given** valid credentials, **When** the user connects, **Then** they reach
   the Tasks tab and the session survives a full reload.
3. **Given** wrong credentials, **When** the user connects, **Then** a
   plain-language error appears and nothing is stored.
4. **Given** an account with 2FA, **When** the user connects without a code,
   **Then** the app asks for the verification code and succeeds once the
   correct code is supplied; a wrong code gets its own distinct error.
5. **Given** a signed-in user, **When** they log out in Settings, **Then** they
   return to login and a reload does not resurrect the session.
6. **Given** a session the NAS has expired, **When** any request fails for that
   reason, **Then** the app returns to the login screen (no dead UI).

---

### User Story 2 - See my downloads live (Priority: P1)

As a signed-in user, I see all my Download Station tasks — name, status,
progress, size, speeds, peers, and time remaining — updating live, with the
NAS's total up/down rate in the header, and pull-to-refresh for an immediate
update.

**Why this priority**: Checking on downloads is the single most frequent thing
a Download Station user does on a phone; together with US1 it is the smallest
shippable product.

**Independent Test**: Seed the (mock) NAS with fixture tasks in known states;
the list renders every fixture with correct fields and reflects task-state
changes within one polling interval.

**Acceptance Scenarios**:

1. **Given** tasks exist on the NAS, **When** the Tasks tab opens, **Then**
   each task shows name, status, percent + size, and (while downloading)
   down/up speed and remaining time.
2. **Given** the tab is open, **When** a task's state changes on the NAS,
   **Then** the list reflects it without any user action within one polling
   interval.
3. **Given** no tasks, **When** the tab opens, **Then** a friendly empty state
   appears (not a spinner forever, not an error).
4. **Given** the app is in the background/hidden, **Then** polling pauses; it
   resumes with an immediate refresh when the app becomes visible.

---

### User Story 3 - Add a download (Priority: P2)

As a signed-in user, I add downloads by pasting one or many URLs (HTTP/FTP/
magnet), or by uploading a `.torrent` file, choose a destination folder by
browsing my NAS shares, optionally provide source credentials or an archive
extract password, and see the new task appear in the list.

**Why this priority**: Adding downloads from the phone is the app's core value
beyond monitoring, but it needs US1+US2 to exist first.

**Independent Test**: From the Tasks tab, open the new-task sheet, submit a
URL and separately a `.torrent` file with a picked destination; both appear as
tasks on the (mock) NAS with the chosen destination.

**Acceptance Scenarios**:

1. **Given** the new-task sheet, **When** the user pastes multiple URLs (one
   per line) and confirms, **Then** one task per valid URL is created and
   non-URL lines are ignored with a visible count of what will be added.
2. **Given** the clipboard holds a URL, **When** the user taps "Paste from
   clipboard", **Then** it is appended to the URL input.
3. **Given** the sheet, **When** the user picks a destination, **Then** they
   can browse shares → subfolders, and the picked path is used for the created
   task.
4. **Given** a `.torrent` file, **When** the user uploads it, **Then** the task
   is created from the file; a file over the size cap is refused with a clear
   message.
5. **Given** no valid URL and no file, **Then** the confirm button stays
   disabled.

---

### User Story 4 - Control tasks (Priority: P2)

As a signed-in user, I pause, resume, and delete tasks from the list, with a
confirmation before anything is deleted.

**Why this priority**: Basic task control completes the monitor→manage loop;
depends on US2's list.

**Independent Test**: With seeded fixture tasks, pause a downloading task,
resume a paused one, and delete one with confirmation; the (mock) NAS state
matches after each action.

**Acceptance Scenarios**:

1. **Given** a downloading task, **When** the user pauses it, **Then** its
   status becomes paused on the NAS and in the list.
2. **Given** a paused task, **When** the user resumes it, **Then** it downloads
   again.
3. **Given** any task, **When** the user chooses delete, **Then** a
   confirmation is required; confirming removes the task from the NAS and the
   list, cancelling changes nothing. Deleting a task never deletes completed
   files on the NAS.

---

### User Story 5 - Filter and sort the list (Priority: P3)

As a user with many tasks, I open a filter sheet to search by term, sort by any
field (creation date, status, size, peers, download speed, upload speed, name,
share ratio, progress, remaining time) ascending or descending, and narrow by
status with multi-select (Finished, Extracting, Finishing, Hash checking,
Downloading, Paused, Stopped, Waiting, File hosting waiting, Moving, Seeding,
Error). My choices persist across app restarts.

**Why this priority**: Quality-of-life on top of US2; the list is useful
without it for small task counts.

**Independent Test**: Seed a diverse fixture set; each sort key orders
correctly both directions, the term filter narrows by name, status multi-select
hides unchecked statuses, and choices survive a reload.

**Acceptance Scenarios**:

1. **Given** mixed tasks, **When** sorting by size descending, **Then** the
   largest task is first; ascending reverses it.
2. **Given** a term, **Then** only tasks whose name contains it
   (case-insensitive) remain.
3. **Given** a status unchecked, **Then** tasks in that status disappear until
   re-checked; "Apply" is what commits the sheet's changes.
4. **Given** filter/sort choices, **When** the app restarts, **Then** the same
   choices are in effect.

### Edge Cases

- NAS unreachable (off, wrong port, TLS failure): login and list surface a
  clear "NAS unreachable" message; the proxy itself stays healthy.
- Session expiry mid-poll (DSM codes 105/106/107/119): one shared path returns
  the user to login; no stacked error toasts from concurrent requests.
- A task with size 0 (magnet still resolving metadata): progress renders 0%,
  no division-by-zero, remaining time shows as unknown.
- Multi-URL input mixing separators (newlines, spaces) and junk text: only
  valid downloadable URLs (http/https/ftp/ftps/magnet/thunder) are extracted.
- Torrent upload over the configured cap is refused client-side AND
  server-side (413) with the same user message.
- Clipboard permission denied: the paste button falls back gracefully (input
  stays usable; no crash).
- Login brute force: repeated failed logins are rate-limited per client IP by
  the proxy.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Users MUST be able to sign in with DSM account + password, with
  an optional 2FA code field that appears when the NAS demands one.
- **FR-002**: The client MUST keep the NAS session id on-device only, sending
  it per request; the server MUST NOT store sessions (constitution III).
- **FR-003**: The login screen MUST show the configured NAS host (hostname
  only) before authentication.
- **FR-004**: Auth failures MUST map to distinct plain-language messages:
  wrong credentials, 2FA required, 2FA invalid, no permission, NAS
  unreachable.
- **FR-005**: `POST /v1/session` MUST be rate-limited per client IP.
- **FR-006**: Users MUST be able to log out, invalidating the NAS session
  best-effort and always clearing local state.
- **FR-007**: The Tasks tab MUST list all tasks with name, status, progress %,
  downloaded/total size, down/up speed, connected peers/seeders, and remaining
  time (while active).
- **FR-008**: The header MUST show the NAS-wide down/up rates.
- **FR-009**: The list MUST poll while visible, pause while hidden, refresh
  immediately on return, and support pull-to-refresh.
- **FR-010**: Any session-expired response MUST return the user to login.
- **FR-011**: Users MUST be able to create tasks from one or many URLs
  (http/https/ftp/ftps/magnet/thunder), extracted from free-form pasted text.
- **FR-012**: Users MUST be able to create a task from a `.torrent` file
  upload, capped by the server's configured size limit (default 16 MiB) with a
  413 on excess.
- **FR-013**: Users MUST be able to pick a destination folder by browsing NAS
  shares and their subfolders; the picked share-relative path is applied to
  created tasks.
- **FR-014**: The new-task sheet MUST offer optional source credentials
  (username/password) for URL tasks and an extract (unzip) password; these are
  forwarded to the NAS and never stored or logged (constitution III).
- **FR-015**: Users MUST be able to pause, resume, and delete tasks; delete
  requires confirmation and never removes completed files.
- **FR-016**: The filter sheet MUST offer term filter, sort by
  creation date / status / size / peers / download speed / upload speed /
  name / share ratio / progress / remaining time, each ascending or
  descending, and a multi-select status filter over the twelve DSM statuses.
- **FR-017**: Filter and sort choices MUST persist on-device across restarts.
- **FR-018**: Sorting and filtering MUST be pure client-side functions with
  unit tests (they never round-trip the NAS).

### Key Entities

- **Session**: sid + account name; lives in the browser (IndexedDB) only.
- **Task**: id, name, type, status, size, downloaded, uploaded, speeds, peers,
  seeders, createdAt, destination — a read-only projection of the NAS's state.
- **Stats**: NAS-wide download/upload rates.
- **Folder**: name + path as shown in the destination picker.
- **Filter state**: term, sort key, sort direction, enabled status set;
  persisted in the settings store.

## Credential-Safety Impact *(constitution-required)*

- **What crosses the proxy**: account+password+OTP once at login (request body
  only); the sid in a header on every call; task URLs, torrent bytes, optional
  source credentials and unzip password at task creation.
- **What is forwarded to the NAS**: exactly the above, over the operator's
  `SYNO_URL`, via the allowlisted APIs (Auth, Task, Statistic, FileStation.List).
- **What could appear in logs/errors**: request method, path, status, duration
  only. Typed errors carry a kind + DSM code, never parameters. No new logging
  is introduced by this spec.
- **Why nothing sensitive is retained**: the server has no store; the sid and
  filter state persist client-side in IndexedDB; passwords are never persisted
  anywhere.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A first-time user goes from opening the app to seeing their task
  list in under 30 seconds (one login form, no manual setup beyond it).
- **SC-002**: Adding a download from a copied link takes at most 4 taps from
  the Tasks tab (add → paste → confirm, destination optional).
- **SC-003**: A task-state change on the NAS is visible in the app within 5
  seconds while the tab is open.
- **SC-004**: 100% of auth failure modes (wrong password, 2FA required, 2FA
  wrong, no permission, NAS down) produce distinct, actionable messages —
  verified by automated tests.
- **SC-005**: The whole flow works against the mock NAS with zero real
  hardware, in CI, on every PR (constitution's dev-parity constraint).

## Assumptions

- Single-NAS deployments: the proxy targets one operator-configured NAS
  (`SYNO_URL`); multi-NAS switching is out of scope.
- DSM 7.x is the primary target; DSM 6 compatibility rides on `SYNO.API.Info`
  discovery and stays best-effort until real-hardware QA.
- Task creation uses the classic Download Station API; per-file selection
  within a torrent ("Select files" in the reference app) needs the newer DSM7
  API and is deferred to a follow-up spec, so the MVP sheet does not show it.
- "Share ratio" sorts by uploaded/downloaded computed client-side; DSM does
  not send a ratio field.
- Remaining time is computed client-side from (size − downloaded) / download
  speed; DSM's own ETA field is not used.
- The reference app's "Live Activity" toggle is iOS-native and out of scope
  for a PWA.

## Clarifications

### Session 2026-07-27

- Q: Should the app auto-relogin with stored credentials when the sid expires?
  → A: No. Storing the password client-side is a security trade-off deferred
  to a dedicated spec; MVP returns to the login screen (account name may
  pre-fill).
- Q: Create-paused toggle in the new-task sheet? → A: Deferred; the classic
  create API has no such flag. Tasks start immediately, matching DSM's default.
- Q: Delete semantics? → A: Delete removes the task only, never completed
  files (`force_complete=false` behavior), matching user expectation from the
  reference app.
- Q: Where does filtering/sorting happen? → A: Client-side over the polled
  list (NAS list sizes are small); keeps the proxy stateless and the logic
  unit-testable.
- Q: Default sort? → A: Creation date, descending (newest first) — matches
  the reference app's default feel; status sort remains available.

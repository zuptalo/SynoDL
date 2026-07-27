# Feature Specification: Stateful multi-user rework — setup wizard, SynoDL accounts, folder access, and Web Push

**Feature Branch**: `feat/0003-stateful-multi-user`

**Created**: 2026-07-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md. -->

**Input**: Operator direction: turn SynoDL into a self-hosted multi-user app — a first-run wizard that
stores the NAS connection + credentials (with 2FA) and a SynoDL admin account in a lightweight
database on a mounted volume; VAPID keys auto-generated on first run; the admin can create app users
who connect without a NAS account, add downloads, and be scoped to specific NAS folders; and opt-in
Web Push notifies users when a download completes (and on app updates) even while they are offline.

## Overview & relationship to the constitution

This feature enacts **Constitution v2.0.0, Principle III (Custodial State & Credential Safety)** — it is
the first spec built on the post-amendment model. SynoDL stops being a stateless proxy and becomes a
self-hosted service with its own accounts and a single encrypted SQLite volume. Everything here MUST
honor the amended Principle III: one volume, secrets encrypted at rest under `SECRETS_KEY`, allowlist-
only NAS access, least privilege, and no secrets in logs.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - First-run setup wizard (Priority: P1)

As the operator deploying SynoDL, the very first time I open it (empty database), I'm guided through a
wizard: I enter the public URL; the NAS address (local or public), port, and whether TLS verification
is required; my NAS credentials (and a 2FA code if the account uses it); and I create the SynoDL admin
account (username + password). SynoDL verifies it can reach and log into the NAS, stores everything
securely, generates its VAPID keys, and drops me at the signed-in admin app. Re-opening never shows
the wizard again.

**Why this priority**: Nothing else works until the instance is configured. It replaces today's
env-var-only configuration (`SYNO_URL`) and the per-client NAS login.

**Independent Test**: Boot with an empty volume → wizard appears → complete it against the mock DSM →
land in the app as admin; restart the container → no wizard, still configured.

**Acceptance Scenarios**:

1. **Given** a fresh install (empty database), **When** I open SynoDL, **Then** the setup wizard is
   shown and the normal app is not reachable until setup completes.
2. **Given** I enter valid NAS details + credentials (with a 2FA code where required), **When** I
   submit, **Then** SynoDL confirms it logged into the NAS, stores the connection, and creates my
   admin account.
3. **Given** the NAS details or credentials are wrong (or a required 2FA code is missing/invalid),
   **When** I submit, **Then** I get a specific, correct error (reusing the existing DSM sign-in error
   messages) and nothing is stored.
4. **Given** setup completed once, **When** I restart the container (same volume), **Then** the wizard
   does not reappear and the stored configuration is used.
5. **Given** setup completed, **When** the wizard route is requested again, **Then** it is unavailable
   (no re-run that could overwrite the NAS connection without an authenticated admin action).

---

### User Story 2 - App users sign in to SynoDL, not the NAS (Priority: P1)

As an app user, I sign in with a SynoDL username and password the admin gave me — I have no NAS
account. Once signed in I see the download tasks and can act on them; SynoDL talks to the NAS for me
using the operator's stored connection.

**Why this priority**: This is the multi-user core — decoupling app access from NAS accounts is the
point of the rework.

**Independent Test**: With an admin-created user, sign in with SynoDL credentials (no NAS account
exists for them) → reach the task list; wrong SynoDL password → rejected.

**Acceptance Scenarios**:

1. **Given** the admin created a user, **When** that user signs in to SynoDL with their username +
   password, **Then** they reach the app without any NAS account of their own.
2. **Given** a signed-in user, **When** they view tasks / add a download / pause-resume-delete, **Then**
   SynoDL performs it on the NAS via the stored connection.
3. **Given** wrong SynoDL credentials, **When** they try to sign in, **Then** they are rejected with a
   clear message and no NAS call is attempted.
4. **Given** a signed-in user, **When** their SynoDL session expires or the admin disables/deletes
   them, **Then** they are returned to the SynoDL sign-in and can no longer act.

---

### User Story 3 - Admin manages users (Priority: P1)

As the admin, I can create, disable, and remove SynoDL users, set/reset their passwords, and mark
whether a user is an admin. Non-admin users cannot reach these controls.

**Why this priority**: Users can't exist without admin management; it gates US2/US4/US5.

**Independent Test**: As admin, create a user, sign in as them elsewhere, disable them, confirm they're
locked out; confirm a non-admin cannot see the admin area.

**Acceptance Scenarios**:

1. **Given** I'm the admin, **When** I create a user with a username + initial password, **Then** that
   user can sign in.
2. **Given** a user exists, **When** I disable or delete them, **Then** they can no longer sign in or
   act, immediately.
3. **Given** I'm a non-admin user, **When** I try to reach user management, **Then** it is not
   available to me.
4. **Given** I'm the admin, **When** I reset a user's password, **Then** the old one stops working and
   the new one works.

---

### User Story 4 - Per-user NAS folder access (Priority: P2)

As the admin, I can restrict which NAS folders a given user may download into; when that user adds a
task, they can only choose (and only succeed in) their allowed folders.

**Why this priority**: Meaningful multi-tenant use needs scoping; without it every user can write
anywhere on the NAS. Marked P2 because US1–US3 deliver a usable single-scope app first.

**Independent Test**: Grant a user access to one folder; as that user the picker shows only that folder
(and its subfolders) and a task-create targeting anything else is refused.

**Acceptance Scenarios**:

1. **Given** the admin scoped a user to folder(s), **When** that user opens the destination picker,
   **Then** only the allowed folder(s) and their descendants are selectable.
2. **Given** a scoped user, **When** a task-create is submitted with a destination outside their
   allowed set (including a crafted request), **Then** SynoDL refuses it before calling the NAS.
3. **Given** a user with no explicit scope, **When** the admin has set a default policy, **Then** that
   policy (e.g. all-folders for admins, none/allowed-set for users) applies.
4. **Given** the admin changes a user's folder access, **When** the user next adds a task, **Then** the
   new scope applies.

---

### User Story 5 - Get notified when a download finishes, even while offline (Priority: P2)

As a user who opted in, I receive a push notification when a download I care about completes — even
when the app is closed / my device was offline — and the notification names what finished. I can opt in
or out per device, and I only get pushes if I opted in.

**Why this priority**: The originally-requested feature; depends on the stateful foundation (stored
subscriptions + a server that learns of completion) from US1–US3.

**Independent Test**: Opt in on a device; with the app closed, drive a seeded download to completion on
the mock; the device receives a push naming the finished task. Opted-out devices receive nothing.

**Acceptance Scenarios**:

1. **Given** I grant notification permission and opt in, **When** a download completes while my app is
   closed, **Then** I receive a push notification that names the finished download.
2. **Given** I have not opted in (or opted out), **When** downloads complete, **Then** I receive no
   push.
3. **Given** a completion happens, **When** it is pushed, **Then** each opted-in device of the relevant
   user(s) is notified once (no duplicate spam, no notification to unrelated users).
4. **Given** a push subscription becomes invalid (browser expired it), **When** SynoDL tries to use it,
   **Then** it is pruned and no error surfaces to users.

---

### User Story 6 - App-update notices (Priority: P3)

As an opted-in user, when a new SynoDL version is deployed, I can receive a plain-text push telling me
an update is available (complementing the existing in-app update prompt).

**Why this priority**: A nice-to-have layered on the US5 push machinery; lowest priority.

**Acceptance Scenarios**:

1. **Given** I'm opted in, **When** the running server version changes from what my device last saw,
   **Then** I receive a plain-text "a new version of SynoDL is available" push.
2. **Given** I'm opted out, **When** a new version deploys, **Then** I receive no push (the in-app
   prompt still works).

---

### Edge Cases

- **NAS session expiry with 2FA**: SynoDL keeps the stored NAS session alive; when it expires and the
  account uses 2FA (a fresh code can't be auto-supplied), SynoDL pauses NAS actions and prompts the
  **admin** to re-enter a 2FA code — users see a clear "temporarily unavailable" state, not errors.
- **Wrong/rotated NAS password** after setup: NAS actions fail cleanly and the admin is guided to
  update the stored connection; no secrets logged.
- **Lost `SECRETS_KEY`**: stored NAS secrets/VAPID can't be decrypted; SynoDL detects this on boot and
  tells the operator to restore the key or re-run setup (documented, not silently reset).
- **Volume missing/unwritable**: SynoDL fails fast with a clear operator message rather than running
  half-configured.
- **First-run race / concurrent wizard submits**: only one setup can complete; the DB enforces a
  single operator config.
- **Push completion detection window**: a download that starts and finishes entirely while nothing is
  watching must still be caught (the completion source must be reliable, not best-effort sampling that
  can miss fast tasks).
- **Folder scope vs. destructive request**: a scoped user crafting a task to a parent/sibling path or
  using path traversal must be refused server-side.
- **iOS**: push requires the PWA be installed to the home screen (documented); opt-in UI reflects when
  the platform can't subscribe.

## Requirements *(mandatory)*

### Functional Requirements

**Setup & configuration**
- **FR-001**: On an empty database SynoDL MUST present a setup wizard and MUST NOT expose the main app
  until setup completes; after completion the wizard MUST NOT be reachable except via an authenticated
  admin re-configuration action.
- **FR-002**: The wizard MUST capture and persist: public URL; NAS address, port, and TLS-verification
  requirement; NAS credentials (with optional 2FA at submit time); and the initial SynoDL admin
  account.
- **FR-003**: The wizard MUST verify the NAS connection (a real login) before storing it, surfacing the
  existing DSM sign-in error messages on failure, and MUST store nothing on failure.
- **FR-004**: SynoDL MUST auto-generate a VAPID key pair on first run and persist it (private key
  encrypted at rest); the public key is exposed to clients for push subscription.

**State & security**
- **FR-005**: All persistent state MUST live in a single SQLite database on one mounted volume; there
  MUST be no other datastore or state location.
- **FR-006**: Stored NAS credentials and the VAPID private key MUST be encrypted at rest under a key
  derived from an operator-provided `SECRETS_KEY`; SynoDL user passwords MUST be stored only as a
  strong salted hash.
- **FR-007**: NAS passwords, OTP codes, DSM session ids, the VAPID private key, SynoDL password hashes,
  and full task URIs MUST never appear in logs, errors, metrics, or panics.

**Accounts & sessions**
- **FR-008**: Users MUST authenticate to SynoDL with a SynoDL username + password (no NAS account),
  receiving a SynoDL session; the NAS is contacted only via the operator's stored connection.
- **FR-009**: Admin users MUST be able to create, disable, delete, and reset the password of users, and
  designate admins; these controls MUST be inaccessible to non-admins (enforced server-side, not just
  hidden in the UI).
- **FR-010**: Disabling or deleting a user MUST take effect immediately (existing sessions invalidated).

**NAS actions on behalf of users**
- **FR-011**: Signed-in users MUST be able to list tasks, add downloads (link/magnet/torrent), and
  pause/resume/delete — SynoDL performing each on the NAS via the stored connection and the existing
  DSM allowlist. No new open-proxy behavior.
- **FR-012**: SynoDL MUST manage the shared NAS session (keep-alive + re-login) transparently; when the
  NAS account uses 2FA and the session expires, it MUST pause NAS actions and prompt an admin to
  re-authenticate rather than failing user actions with raw errors.

**Folder access**
- **FR-013**: The admin MUST be able to scope each user's allowed NAS download folder(s).
- **FR-014**: A user's destination picker MUST show only their allowed folder(s) and descendants, and
  SynoDL MUST validate every task-create's destination against that scope server-side, refusing
  out-of-scope or path-traversal destinations before any NAS call.

**Push notifications**
- **FR-015**: A user MUST be able to grant permission and opt in/out of push per device; SynoDL MUST
  store each device's push subscription and send pushes ONLY to opted-in devices.
- **FR-016**: When a download completes, SynoDL MUST reliably detect it and send a push (naming the
  finished download) to the opted-in devices of the relevant user(s), exactly once per device, and to
  no unrelated users.
- **FR-017**: Push content MUST be plain text; a subscription that the push service reports as gone/
  invalid MUST be pruned silently.
- **FR-018**: Opted-in devices MAY receive a plain-text app-update notice when the deployed version
  changes; opting out suppresses all pushes (the in-app update prompt is unaffected).

**Deploy**
- **FR-019**: SynoDL MUST run from the single container image with just a mounted volume and
  `SECRETS_KEY` (plus nothing else required at first boot) — configurable via `docker compose` or a k8s
  PVC; the deploy docs/manifests MUST reflect the volume + secret.

### Key Entities *(include if feature involves data)*

- **Operator config**: the single NAS connection (address, port, TLS flag, encrypted credentials +
  2FA state) and the public URL. Exactly one per instance.
- **SynoDL user**: username, salted password hash, admin flag, enabled flag, timestamps.
- **Folder grant**: which NAS folder path(s) a user may download into.
- **Push subscription**: a device's Web Push subscription tied to a user, with opt-in state.
- **VAPID keypair**: the instance's push signing keys (private key encrypted).
- **Watched download / completion record**: enough state to detect a task's transition to complete and
  attribute the resulting push to the right user(s) without double-sending.

## Credential-Safety Impact *(constitution-required)*

- **What is stored, and how protected**: the single SQLite volume holds — NAS credentials + VAPID
  private key **encrypted at rest** under `SECRETS_KEY` (env/Secret, never on the volume); SynoDL user
  passwords as **salted hashes** only; push subscriptions, folder grants, and config in the clear
  (non-secret). Download tasks are never persisted (NAS is the source of truth).
- **What crosses to the NAS**: only the existing allowlisted DSM APIs, over the stored NAS connection,
  from the server (users never reach the NAS directly). No new open-proxy behavior; adding any DSM API
  is called out separately.
- **What could appear in logs/errors**: route + outcome + high-level events (login ok/fail, session
  re-auth needed, push sent/pruned) — never NAS passwords, OTP codes, DSM sids, the VAPID private key,
  SynoDL password hashes, or full task URIs.
- **Why this is safe under Principle III (v2.0.0)**: one volume, secrets encrypted at rest, least
  privilege (app users never hold NAS creds; folder scope enforced server-side), allowlist-only NAS
  access. Losing `SECRETS_KEY` renders stored NAS secrets unrecoverable by design.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A fresh deployment is fully usable after completing the wizard once — no env-var NAS
  config required — and never shows the wizard again across restarts on the same volume.
- **SC-002**: An admin can onboard a user who has no NAS account; that user can sign in and add a
  download within minutes, and the download lands on the NAS.
- **SC-003**: A user scoped to a folder can only target that folder; an out-of-scope destination is
  refused before any NAS call, 100% of the time (including crafted requests).
- **SC-004**: An opted-in user reliably receives a push naming the finished download when a download
  completes while their app is closed; opted-out users/devices receive none.
- **SC-005**: No NAS password, OTP, DSM sid, VAPID private key, user password, or full task URI ever
  appears in logs/errors (verified by test + review).
- **SC-006**: The instance runs from one image + one volume + `SECRETS_KEY`; a volume backup + the same
  `SECRETS_KEY` fully restores it.
- **SC-007**: All CI gates stay green, including a REQUIRED checklist for this credential-boundary
  change.

## Assumptions

- **Push completion signal**: the server learns of completion by watching the NAS (a background poll of
  the existing task-list on behalf of the stored connection) — no dependency on the client being open.
  A DSM-webhook source is a possible later optimization, not assumed here. *(This is the concrete
  read of the operator's "notify while not online" requirement.)*
- **NAS 2FA**: the stored NAS account may use 2FA; SynoDL keeps the session alive and re-prompts the
  **admin** on expiry (background code entry is impossible). Operators wanting zero interruptions can
  use a dedicated non-2FA Download-Station account — documented, not required.
- **SQLite via a pure-Go driver** (`modernc.org/sqlite`, no cgo) is the datastore — a deliberate, single
  dependency justified under Governance (needs a real embedded DB); recorded in *Complexity &
  Exceptions* at plan time.
- **Multi-tenancy scope**: all app users act through ONE NAS account, so per-user permissions are
  SynoDL-enforced (DSM cannot express them). "Whose download is it" for push attribution is tracked by
  SynoDL from the task-create it performed.
- **iOS push** requires an installed PWA (documented); the opt-in UI degrades gracefully where the
  platform can't subscribe.
- Reuses the existing DSM 2FA login + typed error mapping (spec 1001) for the wizard's NAS login.

## Out of Scope

- **Per-user NAS accounts / pass-through NAS auth** — the model is deliberately one shared NAS
  connection with SynoDL-side users.
- **DSM-webhook completion push** — a later optimization; this spec uses server-side NAS watching.
- **The Search, Browser, RSS tabs** — separate roadmap specs.
- **The live-updates SSE stream (spec 0002 US1/US3/US4)** — parked; resumes on this new foundation
  afterward. Spec 0002's already-shipped failure-reason is unaffected.
- **Rich/HTML push payloads** — notifications are plain text.

## Clarifications

### Session 2026-07-27

- Q: Datastore? → A: **SQLite via the pure-Go `modernc.org/sqlite` driver** (relaxes zero-dep by one
  justified dependency).
- Q: NAS account & 2FA? → A: **store a 2FA-capable account; re-prompt the admin on session expiry**
  (background re-auth can't supply a fresh code). A dedicated non-2FA account is a documented option.
- Q: Sequencing vs. spec 0002 live-updates? → A: **do this redesign first**; park 0002's SSE work; keep
  its shipped failure-reason.
- Q: Meaning of "update messages in plain text"? → A: **also push app-update notices**, plain text
  (US6), alongside download-complete pushes.

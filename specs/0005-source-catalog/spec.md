# Feature Specification: Download-source catalog — browse, search, and send an admin-configured provider to Download Station

**Feature Branch**: `feat/0005-source-catalog`

**Created**: 2026-07-28

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Operator direction: turn the placeholder Browser tab into a real catalog. An admin configures an
external "download source provider" (the first is a movie/TV site behind bot protection) by pasting session
material captured from their own logged-in browser. SynoDL then calls that provider's API server-side to let
signed-in users browse, search, and quality-filter the catalog, pick a title/quality, and send it to Download
Station — which lands it in a per-title subfolder under the right parent (movies / TV) — reusing SynoDL's
existing folder-and-task flow. Provider credentials are custodial, encrypted, and never reach the client.

## Overview & relationship to the constitution *(review gates)*

This feature extends the **stateful, custodial model** established by Constitution v2.0.0, Principle III and
first enacted in spec 0003. Two aspects require explicit review and a **`/speckit-checklist` pass before
implementation**:

1. **Credential boundary.** Provider session material (a Cloudflare clearance cookie, an app API key, a
   per-user auth token, and the matching User-Agent) is operator-supplied custodial state. It MUST be
   encrypted at rest under the existing `SECRETS_KEY` cipher, be write-only (never returned to any client),
   and never appear in logs or URLs — the same treatment the NAS password already receives.
2. **Outbound allowlist.** Until now the server's only outbound target was the operator's single NAS. This
   feature adds a **second class of outbound target**: the configured provider's API host(s) and its signed
   download hosts. This widening is **operator-opt-in and OFF by default**, and the reachable hosts are
   bounded by the provider configuration (not an open proxy).

To keep the public AGPL project provider-neutral, the capability is modeled as a **generic source-provider
abstraction** (a provider is a name + base URL + a declared set of session fields + an endpoint mapping),
with the first concrete provider supplied as configuration rather than hardcoded site logic in the core.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Admin configures a source provider (Priority: P1)

As the SynoDL admin, I open Settings and add a download-source provider by pasting the session material I
captured from my own logged-in browser on that site (the clearance cookie, API key, auth token, and
User-Agent). SynoDL verifies it can reach the provider and perform a sample catalog call, stores everything
encrypted, and turns the catalog on for everyone. No other user ever sees or handles these values.

**Why this priority**: Nothing in the catalog works until a provider session exists; this is the enabling step
and the sole place credentials enter the system.

**Independent Test**: With an admin account, paste valid session material captured from the same public IP as
the server → SynoDL confirms a successful sample call and saves it; paste invalid/expired material → a
specific error and nothing is stored.

**Acceptance Scenarios**:

1. **Given** I am an admin, **When** I submit valid provider session material, **Then** SynoDL performs a
   sample catalog call, confirms success, stores the material encrypted, and marks the provider active.
2. **Given** I submit material that the provider rejects (bad/expired token, or bot-protection challenge),
   **When** I submit, **Then** I get a specific, non-leaking error and nothing is stored.
3. **Given** a provider is configured, **When** any client reads provider settings, **Then** the secret
   values are never returned — only non-secret status (configured / active / needs-refresh, and when last
   verified).
4. **Given** I am a non-admin user, **When** I look for provider configuration, **Then** it is not available
   to me anywhere in the app.

---

### User Story 2 - Browse and search the catalog (Priority: P1)

As a signed-in user, I open the Browser tab and see the catalog. I search by title and narrow results with the
provider's filters (type — movie / series / anime, quality, genre, language, country). Each result shows its
poster, title, and ratings so I can find what I want.

**Why this priority**: Browsing/searching is the core discovery experience and the visible payoff of the
feature.

**Independent Test**: With a provider active, open the Browser tab → results render with posters and ratings;
apply a quality/genre filter → the result set narrows accordingly; page through results.

**Acceptance Scenarios**:

1. **Given** a provider is active, **When** I open the Browser tab, **Then** I see catalog results with poster,
   title, type, and ratings, and can load further pages.
2. **Given** I enter a search term, **When** I submit, **Then** I see matching titles.
3. **Given** I apply filters (type / quality / genre / language / country), **When** I apply them, **Then** the
   results reflect exactly those filters.
4. **Given** no provider is configured or its session is invalid, **When** I open the Browser tab, **Then** I
   see a clear unavailable/empty state (not an error dump), and — only if I am an admin — a prompt to
   configure/refresh the provider.

---

### User Story 3 - Pick a quality and send it to the NAS (Priority: P1)

As a signed-in user, I open a title, see its available qualities (e.g. "x265 BluRay REMUX 2160p", with size and
resolution), pick one, and tap "Send to NAS". SynoDL creates a subfolder named for the title under the correct
parent folder (movies or TV) and adds the download as a Download Station task there — without me ever thinking
about the site's login.

**Why this priority**: This is the end-to-end value: discovery turns into an actual download on the NAS.

**Independent Test**: Open a released title → pick a quality → Send to NAS → a new subfolder is created under
the correct parent and a task appears in Download Station targeting it.

**Acceptance Scenarios**:

1. **Given** an open title with qualities listed, **When** I pick one and Send to NAS, **Then** a per-title
   subfolder is created under the correct parent and a Download Station task is added targeting that subfolder.
2. **Given** I am a non-admin with folder grants, **When** I Send to NAS, **Then** the destination is only
   accepted if it falls within a folder I am allowed to use; otherwise I am told I cannot download there.
3. **Given** a subfolder for that title already exists, **When** I Send to NAS, **Then** the task is added to
   the existing subfolder rather than failing or duplicating it.
4. **Given** the send succeeds, **When** I check the Tasks tab, **Then** the new download appears and behaves
   like any other Download Station task.

---

### User Story 4 - The provider session expires and the admin refreshes it (Priority: P2)

As a user, when the provider's session has expired, the catalog tells me plainly that "the source needs
refreshing" instead of failing cryptically. As the admin, I get prompted to re-paste fresh session material,
and once I do, the catalog works again for everyone.

**Why this priority**: The captured session is inherently temporary; graceful expiry handling and a one-step
refresh are what make the feature livable rather than a constant support burden.

**Independent Test**: Invalidate the stored session → users see a clear needs-refresh state and admins see a
refresh prompt → admin pastes fresh material → catalog resumes.

**Acceptance Scenarios**:

1. **Given** the stored session has expired, **When** a user opens the catalog or sends a title, **Then** they
   see a clear "source session needs refreshing" state, with no secret values exposed.
2. **Given** the session has expired, **When** the admin opens provider settings, **Then** the provider is
   shown as needs-refresh and the admin can re-paste new material.
3. **Given** the admin submits fresh valid material, **When** it verifies, **Then** the provider returns to
   active and catalog operations resume.

---

### User Story 5 - Remember a preferred quality (Priority: P3)

As a frequent user, I can set a preferred quality so that sending a title uses it by default, minimizing taps.

**Why this priority**: A convenience layer on top of the manual pick; valuable but not required for the MVP.

**Independent Test**: Set a preferred quality → open a title that offers it → the preferred quality is
pre-selected for sending; a title that doesn't offer it falls back to a manual pick.

**Acceptance Scenarios**:

1. **Given** I set a preferred quality, **When** I open a title that offers it, **Then** it is pre-selected.
2. **Given** a title does not offer my preferred quality, **When** I open it, **Then** I am asked to pick from
   what's available.

---

### Edge Cases

- **Cloudflare clearance vs. auth token expiry** are distinct failure modes; both surface the same user-facing
  "needs refreshing" state, but the admin-facing status SHOULD distinguish which layer failed where possible.
- **Public-IP dependency**: the provider binds the session (and signed links) to the public IP that created
  them. If the server's public IP changes (e.g. a dynamic ISP address), the stored session and any not-yet-
  fetched links stop working; this MUST surface as a needs-refresh condition, not a silent hang. *(Documented
  failure mode; see Assumptions.)*
- **Signed-link validity window**: download links expire after a finite window, so they MUST be generated at
  send time and handed to Download Station promptly — never pre-fetched and cached for long.
- **Download host unreachable / link rejected by the NAS**: the send operation reports a clear failure and does
  not leave an empty subfolder as the only trace.
- **Title with no available qualities** (e.g. coming-soon / not yet released): sending is disabled with an
  explanatory message.
- **Series / anime with multiple episodes or seasons** vs. a single movie: the unit of "send" differs; see
  the open question on series scope.
- **Non-admin never reaches credentials**: no client response, log line, or error path may reveal stored
  secret values.
- **Folder-grant conflict**: a non-admin whose grants don't include the mapped parent folder cannot send there
  and is told why.
- **Provider unconfigured / disabled**: the Browser tab degrades to an unavailable state; the rest of the app
  is unaffected.

## Requirements *(mandatory)*

### Functional Requirements

**Provider configuration & credential safety** *(credential boundary — checklist-gated)*

- **FR-001**: Only an admin MUST be able to create, update, refresh, or remove a source-provider configuration.
- **FR-002**: Provider session material (clearance cookie, API key, auth token, User-Agent) MUST be stored
  encrypted at rest under the existing secrets cipher, and MUST be write-only — never returned to any client.
- **FR-003**: The system MUST NOT write provider secrets to logs, error messages, URLs, or query strings.
- **FR-004**: On configure/refresh, the system MUST verify the material with a sample catalog call before
  storing it, and MUST store nothing if verification fails.
- **FR-005**: The system MUST expose only non-secret provider status to clients (e.g. not-configured /
  active / needs-refresh, plus when it was last verified).
- **FR-006**: The source-catalog capability MUST be OFF by default and active only once an admin configures a
  provider.

**Catalog access (server-side integration)**

- **FR-007**: The system MUST perform all provider API calls server-side using the stored session material;
  clients MUST never call the provider directly.
- **FR-008**: Outbound provider access MUST be limited to the hosts declared by the provider configuration
  (its API host(s) and signed download host(s)); it MUST NOT act as an open/arbitrary proxy.
- **FR-009**: The provider integration MUST be modeled generically (name + base URL + declared session fields
  + endpoint mapping) so additional providers can be added as configuration, with the first provider supplied
  as such rather than hardcoded into core logic.
- **FR-010**: The system MUST support catalog search and browsing with the provider's filters — type
  (movie / series / anime), quality, genre, language, country — and paginated results.
- **FR-011**: Catalog results MUST include, per title, at least: title, type, poster image, and available
  ratings.
- **FR-012**: Opening a title MUST list its available download qualities, each with at least a quality label,
  file size, and resolution.

**Send to Download Station**

- **FR-013**: Sending a chosen quality MUST create (or reuse) a per-title subfolder under the parent folder
  appropriate to the title's type, and add the download as a Download Station task targeting that subfolder,
  reusing the existing folder-creation and task-creation flow.
- **FR-014**: The signed download link MUST be generated at send time and handed to Download Station promptly
  (not cached long), because it expires.
- **FR-015**: The destination MUST honor per-user folder grants: a non-admin may only target a folder within
  their grants; a disallowed destination MUST be refused with a clear reason.
- **FR-016**: If a per-title subfolder already exists, the system MUST add the task to it rather than
  duplicating or failing.

**Availability, expiry & UX**

- **FR-017**: The Browser tab MUST present the catalog only when a provider is active; otherwise it MUST show a
  clear unavailable/empty state rather than an error dump.
- **FR-018**: When provider calls fail due to an expired/invalid session, users MUST see a clear "source
  session needs refreshing" state and admins MUST be prompted to re-paste, with no secret exposure.
- **FR-019**: A public-IP mismatch (session/link bound to a different IP than the server now uses) MUST surface
  as a needs-refresh condition, not a silent failure.
- **FR-020**: Users MAY set a preferred quality that is pre-selected when a title offers it, falling back to a
  manual pick otherwise. *(P3)*

### Key Entities *(include if feature involves data)*

- **Source Provider**: an operator-configured integration target — display name, base/API host(s), declared
  signed-download host(s), the set of session fields it requires, its endpoint mapping, and enabled/disabled
  state. Neutral of any specific site.
- **Provider Session**: the encrypted, write-only custodial secrets for a provider (clearance cookie, API key,
  auth token, User-Agent) plus non-secret status metadata (state, last-verified time).
- **Catalog Title**: a discoverable item — id, type (movie / series / anime), title, poster, ratings, and
  facets used for filtering.
- **Quality Option**: a downloadable variant of a title — quality label, size, resolution (and other
  descriptors the provider offers), resolvable to a signed download link at send time.
- **Send Request → Download Task**: the mapping of a chosen title+quality to a Download Station task in a
  per-title subfolder under the type-appropriate parent, subject to the user's folder grants.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An admin can configure a working provider (paste → verify → active) in under 3 minutes, with a
  clear pass/fail result.
- **SC-002**: With an active provider, a user can go from opening the Browser tab to a title downloading on the
  NAS in **5 taps or fewer** for the common case.
- **SC-003**: Applying any provider filter (type / quality / genre / language / country) returns a result set
  consistent with that filter on **100%** of applications.
- **SC-004**: Provider secret values appear in **zero** client responses and **zero** log lines across all
  flows (verified by inspection/tests).
- **SC-005**: A non-admin can perform **zero** provider-configuration actions and can target **zero** folders
  outside their grants.
- **SC-006**: When the session is valid, catalog browse/search/send operations succeed in at least **95%** of
  attempts (excluding provider-side outages).
- **SC-007**: When the session is expired or the IP no longer matches, **100%** of affected operations show the
  "needs refreshing" state rather than a raw error or an indefinite hang.
- **SC-008**: A per-title send results in the download landing in a correctly named subfolder under the
  type-appropriate parent in **100%** of successful sends.

## Assumptions

- **Same, stable public IP**: the SynoDL server and the NAS share one public egress IP, and it is the same IP
  the admin used to capture the session material. The provider binds sessions and signed links to that IP, so
  a stable (effectively static) public IP is assumed; a rotating IP is a known limitation surfaced as
  needs-refresh (Edge Cases, FR-019).
- **Admin has an active account on the provider**: SynoDL does not create provider accounts or automate login;
  the admin supplies session material captured from their own authenticated browser.
- **Operator responsibility for provider terms/legality**: enabling a provider and the legality of downloading
  from it are the operator's responsibility; the capability ships off by default and provider-neutral.
- **Signed links are self-authenticating and fetchable by the NAS** (IP + expiry + signature embedded), served
  from a host the NAS can reach directly without cookies/token, with a validity window measured in the range of
  days-to-weeks.
- **Reuse of existing flows**: destination subfolder creation and task creation reuse SynoDL's existing
  folder/task capabilities and per-user folder-grant enforcement; no new download mechanism is introduced.
- **First provider supplied as configuration**: the first concrete provider (a movie/TV site) is delivered as
  provider configuration/data, not as hardcoded site logic, keeping the core provider-neutral.
- **Parent-folder mapping** *(informed default; open to refinement in `/speckit-clarify`)*: the admin
  designates, per provider, the parent folders for each title type (e.g. a movies parent and a TV parent); the
  feature selects the parent from the title's type and creates the per-title subfolder beneath it.

## Resolved Decisions *(locked for v1)*

- **Series/anime send scope → movies-first.** In v1, **movies** are supported end-to-end (browse → search →
  pick quality → Send to NAS). **Series and anime remain fully browsable and searchable**, but "Send to NAS"
  is not offered for them in v1 (their download/quality shape — per-season vs. per-episode — needs its own
  provider-response capture and design). The parent-folder mapping still provisions a TV/series parent so
  series-send drops in as a fast-follow without rework. *(Scopes US3 to movies; series/anime satisfy US2 only.)*
- **Preferred-quality ownership → per-user.** FR-020's preferred quality is a **per-user** setting (stored
  per user like destination preferences), auto-selected when the opened title offers it and falling back to a
  manual pick otherwise. No admin-wide default in v1.

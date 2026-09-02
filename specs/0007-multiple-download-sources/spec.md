# Feature Specification: Multiple Download Sources

**Feature Branch**: `feat/0007-multiple-download-sources`

**Created**: 2026-09-02

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "Introduce a second download source (zarfilm.com) alongside the
existing one. By default Discover shows all sources combined, with a dropdown to narrow to a
single source. Set up local dev so both the existing source and the new one can be exercised
for real."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Browse every configured source at once (Priority: P1)

An operator has configured more than one download source. A user opens Discover and sees
titles from all of them in one list, without having to know or care which site each came from.
A source dropdown sits in the toolbar beside the sort control, showing "All sources" by
default; picking a single source narrows the list to just that one. The choice is remembered
the next time they open Discover.

**Why this priority**: Everything else in this feature depends on the app being able to hold
more than one source at a time. Today a source is a singleton — one stored configuration, one
implicit source behind every catalog request — so no second source can exist until this is
lifted. It is also independently valuable: it is the entire user-visible payoff.

**Independent Test**: Configure the SAME provider kind twice as two separate sources, or one
real source plus the mock source. Discover shows results from both interleaved, the dropdown
lists both plus "All sources", and selecting one filters the list to it. No second provider
driver is required to test this.

**Acceptance Scenarios**:

1. **Given** two sources are configured and healthy, **When** the user opens Discover,
   **Then** the list contains titles from both sources and the dropdown reads "All sources".
2. **Given** the user is viewing "All sources", **When** they pick a single source from the
   dropdown, **Then** the list reloads showing only that source's titles, in that source's
   exact sort order.
3. **Given** the user selected a single source, **When** they leave Discover and return later
   (including after an app restart), **Then** that source is still selected.
4. **Given** two sources are configured, **When** the user scrolls to load more results in
   "All sources" mode, **Then** further results from both sources are appended and no title
   already on screen is repeated.
5. **Given** the user opens a title from the combined list, **When** the detail opens,
   **Then** it shows that title's own source's download options and sending works, with no
   ambiguity about which source the title came from.
6. **Given** only one source is configured, **When** the user opens Discover, **Then** the
   experience is unchanged from today and the dropdown is either hidden or shows that one
   source.

---

### User Story 2 - Add zarfilm.com as a second source (Priority: P2)

An operator adds zarfilm.com as a source by pasting session material captured from a browser
where they have logged in (its login requires a human-solved captcha, so it cannot be
automated). Once verified, zarfilm titles appear in Discover alongside the existing source and
can be sent to Download Station like any other.

**Why this priority**: This is the concrete second source that motivates the work, but US1
must land first for it to have anywhere to live. It is also the story that proves the
provider abstraction actually generalizes beyond the site it was written for.

**Independent Test**: With US1 in place, configure zarfilm as an additional source, verify
the session, browse and search its catalog, open a title, and send one quality to Download
Station. Testable on its own against the real site with a live session, and against the mock
source without one.

**Acceptance Scenarios**:

1. **Given** an operator has pasted valid zarfilm session material, **When** they save it,
   **Then** the source verifies and reports itself active without the operator entering a
   username, password, or captcha into SynoDL.
2. **Given** an active zarfilm source, **When** a user browses or searches Discover,
   **Then** zarfilm titles appear with poster, title, IMDb identifier and rating, matching
   the presentation of existing sources.
3. **Given** a user opens a zarfilm title, **When** the detail loads, **Then** its download
   options are listed with their size, encoder, and whether the release is Persian-dubbed or
   Persian-subtitled, distinguishable from one another.
4. **Given** a user picks a zarfilm download option, **When** they send it, **Then** the task
   appears in Download Station and downloads to completion without SynoDL supplying any
   session material to the NAS.
5. **Given** the pasted session belongs to an account with no active subscription, **When**
   the source is verified, **Then** it reports that distinct condition — the operator is told
   the account cannot download, not that the session is invalid.
6. **Given** the stored session has expired, **When** a user browses Discover, **Then** the
   source reports that it needs refreshing and an admin is prompted to paste fresh material.

---

### User Story 3 - Filters that mean the same thing everywhere (Priority: P3)

In "All sources" mode the filter sheet offers only the filters every enabled source
understands, so anything the user applies genuinely applies to every result on screen.
Selecting a single source reveals that source's own extra filters.

**Why this priority**: Filtering is already usable without this — US1 can ship offering the
existing filter set. This story removes the confusion of applying a filter that silently only
affects part of the list, which becomes visible once two sources with different capabilities
are live.

**Independent Test**: With two sources whose available filters differ, confirm that the
combined filter sheet shows only the shared filters, and that switching to a single source
adds that source's extras and removes them again on switching back.

**Acceptance Scenarios**:

1. **Given** two sources with differing filter capabilities in "All sources" mode, **When**
   the user opens the filter sheet, **Then** only filters both sources support are offered.
2. **Given** the user selects a single source, **When** they open the filter sheet, **Then**
   that source's own additional filters are offered as well.
3. **Given** the user has applied a source-specific filter, **When** they switch back to "All
   sources", **Then** that filter is dropped and the user can see it was dropped.
4. **Given** filter options with the same meaning but different labels across sources,
   **When** shown in combined mode, **Then** each option appears once.

---

### User Story 4 - Both sources exercisable in local development (Priority: P2)

A developer can run the whole feature locally two ways: against fake source sites that need no
credentials and no internet, and — when they have live session material — against the real
sites, to catch drift the fakes cannot.

**Why this priority**: Equal to US2 in practice. Without the credential-free path this feature
gets no automated coverage at all, because the end-to-end stack runs without stored state and
cannot hold a real session. Without the live path, a mock written from captured markup would
quietly diverge from the real sites.

**Independent Test**: With no credentials and no network access to the real sites, start the
dev stack and complete the US1 and US2 journeys against the fakes. Separately, with live
session material supplied through the environment, run the live checks against the real sites.

**Acceptance Scenarios**:

1. **Given** a developer with no source credentials and no access to the real sites, **When**
   they start the local dev stack, **Then** they can configure the fake sources, browse,
   search, open a title, and send a download.
2. **Given** the automated end-to-end suite, **When** it runs in continuous integration,
   **Then** it covers the combined-sources journey without any real credential.
3. **Given** a developer with live session material, **When** they run the live checks,
   **Then** each real source is verified, browsed, searched, and a download link resolved.
4. **Given** no live session material is present, **When** the test suite runs, **Then** the
   live checks skip rather than fail.

---

### Edge Cases

- **One source unhealthy, others fine.** In "All sources" mode a source whose session has
  expired, whose site is unreachable, or which times out MUST NOT blank the view: healthy
  sources still render, with a non-blocking notice naming the unhealthy one and offering the
  admin path to fix it. Selecting that source alone shows its error in full.
- **All sources unhealthy.** The view reports the failure once, not once per source.
- **No sources configured.** Unchanged from today's not-configured experience.
- **Sources exhaust at different rates.** One source runs out of pages long before another;
  loading more continues from the sources that still have results, and the list only ends when
  every source is exhausted.
- **The same title on two sources.** Both sites carry much of the same catalog, so combined
  results will contain the same film twice from different sources. These are listed as
  separate entries, each labelled with the source it came from, rather than collapsed into
  one — what the user sees is exactly what each source offers. Popular titles will therefore
  repeat, most visibly on the first page of a "newest" browse; the source label is what makes
  that legible rather than looking like a bug.
- **A source is removed while a user is browsing it.** The user is returned to "All sources"
  rather than left on a dead selection.
- **A source is disabled but not deleted.** Its titles leave the combined list and it does not
  appear in the dropdown, but its configuration survives.
- **Signed download links expire.** Links from the new source expire in roughly a day, so a
  link MUST be resolved at send time rather than reused from when the title was viewed.
- **Signed links may be bound to the requesting network.** Unverified at spec time: links from
  the new source were confirmed fetchable without session material, but not from a different
  public address. If they turn out to be address-bound, downloads would fail only when SynoDL
  and the NAS sit behind different addresses. This MUST be tested against a real NAS during
  implementation and, if confirmed, reported to the operator as a distinct failure.
- **Text search versus browse.** Sources may support fewer sort options during text search
  than while browsing; combined search must not silently claim a sort it cannot honour.
- **A subscription lapses mid-session.** A previously-active source starts returning no
  download options; treated as the unsubscribed state, not as an expired session.

## Requirements *(mandatory)*

### Functional Requirements

**Multiple sources**

- **FR-001**: The system MUST allow an operator to configure more than one download source at
  the same time, including two instances of the same source kind.
- **FR-002**: Each configured source MUST carry its own display name, session material,
  enabled flag, health state, download-size policy, and destination folders, independent of
  every other source.
- **FR-003**: Administrators MUST be able to list, add, edit, enable, disable, and remove
  individual sources without disturbing the others.
- **FR-004**: An existing single configured source MUST survive the upgrade unchanged,
  becoming the first entry in the list, with no reconfiguration and no re-pasting of session
  material.
- **FR-005**: Every catalog result, title, and download option MUST be unambiguously
  attributable to the source it came from, so that acting on it addresses that source.
- **FR-005a**: A title carried by more than one source MUST appear once per source rather than
  being merged into a single entry.

**Combined Discover**

- **FR-006**: Discover MUST default to showing results from all enabled, healthy sources
  combined.
- **FR-007**: Discover MUST offer a source selector listing "All sources" plus each enabled
  source; selecting one MUST narrow the results to that source alone.
- **FR-008**: The selected source MUST persist across sessions and devices alongside the
  user's other saved Discover view state.
- **FR-009**: In combined mode, results MUST be drawn from each source in turn so that every
  source is represented on the first screenful; ordering is exact within a source and
  approximate across sources.
- **FR-010**: In single-source mode, ordering MUST be exactly what that source returns.
- **FR-011**: Loading further results in combined mode MUST NOT repeat titles already shown
  and MUST continue drawing from sources that still have results after others are exhausted.
- **FR-012**: A failing source MUST NOT prevent healthy sources from rendering; the failure
  MUST be surfaced non-blockingly, naming the affected source.
- **FR-012a**: In combined mode every result MUST visibly indicate which source it came from,
  so that a title appearing more than once reads as two sources offering it rather than as a
  duplicate. In single-source mode this indicator is redundant and MUST NOT clutter the list.
- **FR-013**: When only one source is configured, the Discover experience MUST be equivalent
  to today's.

**Filters**

- **FR-014**: In combined mode the filter sheet MUST offer only filters supported by every
  enabled source.
- **FR-015**: In single-source mode the filter sheet MUST additionally offer that source's own
  filters.
- **FR-016**: Switching from a single source back to combined mode MUST drop filters the other
  sources cannot honour, visibly rather than silently.

**The new source**

- **FR-017**: The system MUST support zarfilm.com as a configurable source kind.
- **FR-018**: An operator MUST be able to authorize it by pasting session material captured
  from an already-logged-in browser; SynoDL MUST NOT ask for the site username, password, or
  captcha, and MUST NOT attempt to log in on the operator's behalf.
- **FR-019**: The system MUST verify pasted session material before accepting it and report
  the outcome as one of: working, not logged in, logged in but unable to download (no active
  subscription), or unreachable.
- **FR-020**: The system MUST present the source's titles with poster, title, type, IMDb
  identifier and rating, and plot where available.
- **FR-021**: The system MUST present each download option with its size, encoder, resolution,
  and whether the release is dubbed or subtitled.
- **FR-022**: The system MUST resolve a download link at the moment of sending, never reusing
  a link obtained earlier.
- **FR-023**: Download links handed to Download Station MUST be usable by the NAS without any
  session material, credential, or header from SynoDL.
- **FR-024**: Outbound requests to the new source MUST be confined to its declared host
  allowlist, extended by spec-level decision only, exactly as for existing sources.

**Development and test parity**

- **FR-025**: The local development stack and the automated end-to-end suite MUST be able to
  exercise multiple sources with no real credentials and no access to the real sites.
- **FR-026**: The automated end-to-end suite MUST cover the combined-sources journey.
- **FR-027**: Checks against the real sites MUST be runnable on demand with supplied session
  material, and MUST skip — not fail — when it is absent.
- **FR-028**: No real credential or session value may be committed to the repository or
  required by continuous integration.

**Project constraints**

- **FR-029**: Introducing the server's first third-party dependency MUST be recorded in the
  project documents that currently assert the server has none, so the constraint reads as
  deliberately amended rather than violated. Any further server dependency remains a
  spec-level decision, not an implementation detail.

### Key Entities

- **Source**: One configured download site instance. Has a kind (which site it is), a display
  name, enabled state, health state, destination folders, a size policy, and its own outbound
  host allowlist. There may be many; two may share a kind.
- **Source session**: The secret material proving an operator's authorization to one source.
  Shape differs by kind — the existing source uses request headers, the new one a browser
  login cookie. Write-only: stored encrypted, never returned to any client.
- **Catalog title**: One item in Discover, always carrying the identity of the source it came
  from.
- **Download option**: One downloadable variant of a title, belonging to one title on one
  source, whose actual link is obtained only at send time.
- **Discover view state**: A user's remembered Discover selections — now including the chosen
  source or "All sources" — alongside filters and sort.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add a second source and see its titles in Discover without
  reconfiguring, re-authorizing, or restarting the existing one.
- **SC-002**: With two healthy sources, the first screenful of combined Discover contains
  titles from both.
- **SC-003**: Switching between "All sources" and a single source takes one interaction and
  shows updated results without leaving Discover.
- **SC-004**: With one source failing and one healthy, users still see the healthy source's
  results, and can tell which source is having trouble.
- **SC-005**: Combined Discover loads its first screenful no more than 50% slower than
  single-source Discover does today.
- **SC-006**: A download sent from the new source completes on the NAS without a stored
  session being shared with it.
- **SC-007**: A developer with no credentials and no access to the real sites can complete
  every user journey in this spec against the local stack.
- **SC-008**: Every user journey in this spec except the live-site checks is covered by
  automated tests that run in continuous integration with no real credential.
- **SC-009**: An operator whose session has expired is told which source needs attention and
  what to do, without needing to read a log.
- **SC-010**: No session value, cookie, or signed link appears in any log line, error
  response, or metric, verified by test.

## Credential-Safety Impact

Required by constitution Principle III.

- **What is stored, and how it is protected.** One session record per configured source,
  sealed with the at-rest cipher under the operator-provided secret, exactly as the existing
  single source's material is today. Write-only: an administrator may replace it but never
  read it back. Removing a source destroys its session material.
- **Elevated sensitivity of the new source's material.** The new source authorizes with a site
  login cookie, which grants everything that site account can do — broader than the existing
  source's scoped API token, which authorizes only catalog calls. Additionally, every signed
  download link it produces embeds the account identifier, so a leaked link identifies the
  account. This raises the consequence of a leak without changing the storage mechanism, and
  MUST be stated plainly to the operator at the point where they paste it, including that the
  material should be revoked at the site if they suspect exposure.
- **What crosses to the NAS.** Only a resolved download link and its destination folder — the
  same as today. No session material, cookie, or header from any source is ever forwarded to
  the NAS, and the NAS never learns which source a link came from beyond what the link itself
  reveals. Verified by the requirement that links work with no session material at all
  (FR-023).
- **What could appear in logs or errors.** Session values, cookies, and signed download links
  MUST NOT appear in log lines, error payloads, metrics, or panics — signed links included,
  because they embed the account identifier and grant unauthenticated access until they
  expire. Errors returned to clients carry a category (needs refresh, unsubscribed,
  unreachable) and the source's display name, never upstream response bodies. Existing
  redaction rules extend unchanged to the second source; the added risk is a new code path
  that could log a URL, which the tests must cover.
- **Outbound surface.** Two hosts are added to the allowlist for the new source: its own site,
  and the domain its signed download links are served from (matched by domain suffix, since
  the link host rotates across numbered subdomains). Its poster host is added to the image
  proxy's allowlist. No source may be reached at a host a client supplies; allowlists remain
  provider-declared and spec-governed.
- **Why.** The operator is authorizing SynoDL to act as them on a site they hold an account
  with. Custody must therefore match the existing NAS-credential standard, and the wider blast
  radius of a full login cookie must be disclosed rather than absorbed silently.

## Assumptions

- **Operator-opt-in, off by default.** Sources remain something an operator configures
  deliberately; a fresh install has none, as today.
- **Scale.** A handful of sources at most (single digits). Combined browsing is not designed
  for dozens, and no paging strategy is optimized for that case.
- **Sources are catalog-equivalent.** Every source can be browsed, searched, and can produce a
  direct link. A source that could not would not fit this model.
- **Per-user preferences stay global.** A user's preferred quality and any download quota
  apply across sources rather than per source; destination folders and size policy stay
  per-source, as they are today.
- **Sending remains one option at a time.** Combining sources does not introduce sending the
  same title from several sources at once.
- **The existing source's behavior is unchanged.** Its ordering, filters, and download flow
  stay as they are when it is the only source selected.
- **Session lifetime differs per source.** The new source's material lasts roughly two weeks,
  against a much shorter life for the existing source's; refresh prompts are per-source and
  should not imply all sources expired together.
- **The new source's catalog markup is not a published interface.** It is a website, so its
  structure can change without notice; breakage is expected to surface as a source-level
  error rather than a crash, and the live checks exist to catch drift early.
- **The server takes its first third-party dependency.** Until now the server carried none.
  Reading the new source's catalog means interpreting web pages rather than a structured data
  feed, so this spec accepts a real markup tokenizer rather than pattern-matching over raw
  markup: shorter, clearer, and far more tolerant of the source changing its page structure.
  The dependency is limited to a Go-team-maintained module. This ends a stated project
  property, so the change MUST be recorded where that property is documented rather than
  slipped in — see FR-029.
- **Live checks are a developer tool, not a gate.** They run on demand against the real sites
  and never in continuous integration, which has neither credentials nor a stable address.

## Dependencies

- Builds directly on the existing source-catalog feature (spec 0005): its provider
  abstraction, image proxy, per-user quota, and admin session-paste flow are all reused.
- Requires an operator-held account on the new source with an active subscription for the
  live path; the credential-free path requires nothing.
- The new source's signed links must be reachable from the NAS itself; if they prove to be
  address-bound this constrains deployments where SynoDL and the NAS egress differently.

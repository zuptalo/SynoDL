# Feature Specification: Who made it — cast and director on a title

**Feature Branch**: `feat/0014-cast-and-director-title`

**Created**: 2026-09-16

**Status**: planned
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "have a look at both sources and see if they are providing the casts and the director information for their titles … let's also pull their images from IMDb if they don't already exist in the sources and make their tile a link to their IMDb profile … if we already have seen the cast in one title and we land on another one with the same cast member, the images for the cast can be reused from the cache"

## Overview

Opening a title in Discover tells the user what it is and what they can download,
but not **who is in it**. Spec 1019 answered that question by sending them away:
the IMDb rating is a link, and "cast, reviews, whether it's worth it" was
explicitly somebody else's page. That was the right call when SynoDL had no cast
data of its own. It no longer is — both configured sources publish cast and
director for every title, and SynoDL throws it away on the way through.

This feature keeps it. Below the download options, the detail sheet gains the
people who made the title: the cast, with the character each plays where the
source says so, and the crew — director, creator, writers — as far as each source
publishes them. Each person is a tile with their face on it, and tapping the tile
still goes to IMDb — to *that person's* page, which is the thing the user was
actually reaching for.

The complication is that the sources are uneven about photographs. One publishes
a person's photo sometimes and a grey placeholder the rest of the time, and never
publishes one for a director at all; the other publishes none on the title page
and one on the person's own page. So a face has to be assembled: use what the
source has, and where it has nothing, fall back to IMDb — which is also where the
source's own re-hosted stills came from. That fallback is a new outbound host and
therefore a deliberate widening of SynoDL's outbound surface, which is why this
spec carries a Credential-Safety Impact section and a security checklist.

Because people repeat across titles far more than titles repeat across users, a
resolved face is cached against the **person**, not the title. The second title
this evening with the same lead actor must not cost a second lookup, and a
restart must not throw the work away.

## Clarifications

### Session 2026-09-16

- Q: One source leaves a series' director empty and publishes a creator and writers instead — which roles should the section cover? → A: Cast, plus director, creator, and writers — every "who made it" role either source publishes.
- Q: Where do the people sit on the detail sheet relative to the download options? → A: Below the download options, so sending a download stays the sheet's first action.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See who is in it, without leaving (Priority: P1)

A user opens a title in Discover. Below the download options, the sheet names the
people who made it: the cast in the order the source bills them, each with the
character they play where the source publishes one, and behind them the crew —
the director, the creator, the writers — as far as the source publishes each.

**Why this priority**: It is the whole feature. Every other story decorates this
one; without it there is nothing to decorate. It is also the part that costs
nothing new — both sources already answer it — so it must stand alone even if
every photo lookup in the rest of this spec fails.

**Independent Test**: Open a title from each configured source against the mock
sites and confirm the cast names, character names, and director appear, with no
image resolution involved at all.

**Acceptance Scenarios**:

1. **Given** a title whose source publishes a cast, **When** the user opens its
   detail sheet, **Then** the cast is listed in the source's billing order with
   each person's name.
2. **Given** a source that publishes the character a person plays, **When** the
   cast is shown, **Then** each person's character is shown with their name.
3. **Given** a source that publishes no character names, **When** the cast is
   shown, **Then** the names are shown alone — never an empty line, a dash, or a
   guess.
4. **Given** a title with one or more directors, **When** the sheet is open,
   **Then** the director is shown, labelled as such and distinct from the cast.
5. **Given** a series whose source leaves the director empty but publishes a
   creator, **When** the sheet is open, **Then** the creator is shown under its own
   label — the section is not blank merely because nobody is called "director".
6. **Given** a source that publishes writers, **When** the sheet is open, **Then**
   they are shown under their own label, after the director and creator.
7. **Given** a title whose source publishes no people at all, **When** the sheet
   is open, **Then** no people section appears — not an empty heading.
8. **Given** the source fails to answer the question of who made a title,
   **When** the sheet is open, **Then** the download options still render and the
   sheet is otherwise unchanged.

---

### User Story 2 - Faces, not a list of names (Priority: P1)

Each person is a tile showing their photograph, so the user recognises a cast at
a glance instead of reading it.

**Why this priority**: Recognition is the point of showing a cast at all; a
column of text is a worse version of the IMDb link that already existed. It is P1
with US1 because the tile is the unit of the design — but it degrades, per US5,
rather than blocking.

**Independent Test**: Open a title whose source publishes real photographs and
confirm each tile shows one, served same-origin.

**Acceptance Scenarios**:

1. **Given** a person whose source publishes a real photograph, **When** the tile
   renders, **Then** it shows that photograph.
2. **Given** a source that publishes a stand-in image rather than a photograph of
   the person, **When** the tile renders, **Then** the stand-in is never shown as
   though it were their photo.
3. **Given** a list of people longer than the screen, **When** the sheet renders,
   **Then** the list is browsable without the sheet itself scrolling sideways.

---

### User Story 3 - A missing face is filled in from IMDb (Priority: P2)

Where a source has no photograph of a person — the common case for a director,
and frequent for supporting cast — SynoDL fetches one from IMDb, if IMDb has one.

**Why this priority**: It is the difference between a row of faces and a row of
mostly-blanks: on one source, every director and roughly half of a typical cast
arrive with no photo. It is P2 because US1 and US2 are useful without it.

**Independent Test**: Open a title where every person's source image is a
stand-in, and confirm the tiles fill in from the fallback while the sheet itself
never waits for it.

**Acceptance Scenarios**:

1. **Given** a person with no source photograph but a known IMDb identity,
   **When** their tile renders, **Then** their IMDb photograph is shown.
2. **Given** a person IMDb has no photograph of, **When** their tile renders,
   **Then** it falls back to the readable placeholder of US5 and is not retried on
   every subsequent view.
3. **Given** the fallback lookup is slow, unreachable, or refuses SynoDL,
   **When** the sheet is opened, **Then** the names, characters and links are
   unaffected and appear at their normal speed.
4. **Given** a person whose source photograph is real, **When** their tile
   renders, **Then** no fallback lookup is made for them at all.

---

### User Story 4 - The tile is the way to their IMDb page (Priority: P2)

Tapping a person opens their IMDb profile, in the same manner as the existing
IMDb rating link on the same sheet.

**Why this priority**: It replaces what spec 1019 could only gesture at — the
user wanting to know more about a person, rather than about the title — and it
costs nothing beyond what the photo lookup already establishes.

**Independent Test**: Tap a person tile and confirm it opens that person's IMDb
page externally, leaving the sheet as it was.

**Acceptance Scenarios**:

1. **Given** a person whose IMDb identity is known, **When** the user taps their
   tile, **Then** that person's IMDb page opens externally.
2. **Given** a person whose IMDb identity is not known, **When** their tile
   renders, **Then** it is plainly not a link and tapping it does nothing.
3. **Given** the user returns from IMDb, **When** they come back, **Then** the
   sheet is still open on the same title.

---

### User Story 5 - Nobody renders as a broken image (Priority: P2)

A person with no photograph anywhere still gets a tile: their initials, in the
app's own styling, the same size and shape as everyone else's.

**Why this priority**: Both sources have people with no photo, and IMDb has
people with no photo. A broken-image glyph or a collapsed tile would make the
common case look like a bug.

**Independent Test**: Open a title where no photograph exists for anyone and
confirm a tidy row of initials tiles, with the layout identical to a row of
photos.

**Acceptance Scenarios**:

1. **Given** a person with no photograph from any origin, **When** their tile
   renders, **Then** it shows their initials and their name, never a broken
   image.
2. **Given** a photograph that fails to load in the browser, **When** it fails,
   **Then** the tile falls back to the initials rather than leaving a gap.
3. **Given** a mixed cast of photographed and unphotographed people, **When** the
   row renders, **Then** every tile is the same size and alignment.

---

### User Story 6 - The same actor in the next title is already known (Priority: P2)

A person resolved once is resolved for good: opening another title they appear in
shows their face immediately, and a server restart does not undo that.

**Why this priority**: It is what makes the fallback affordable. Without it, a
user browsing an evening's worth of titles re-fetches the same handful of famous
people dozens of times, against a third party that will eventually refuse. The
user asked for it explicitly.

**Independent Test**: Open two different titles sharing a cast member, and
confirm the second title resolves that person with no further outbound lookup;
restart the server and confirm it still does.

**Acceptance Scenarios**:

1. **Given** a person whose photograph was resolved while viewing one title,
   **When** the user opens a different title featuring them, **Then** the
   photograph is served without a new lookup to the third party.
2. **Given** a person previously found to have no photograph, **When** they
   appear in another title, **Then** no new lookup is made for them until the
   remembered answer expires.
3. **Given** the server restarts, **When** a previously-resolved person appears
   again, **Then** their photograph is still known.
4. **Given** a remembered answer has aged past its lifetime, **When** that person
   is next shown, **Then** the answer is refreshed rather than trusted forever.

---

### User Story 7 - The detail sheet learns what it was missing (Priority: P3)

While it is being asked who made a title, one source also answers questions the
sheet has been leaving blank: the synopsis, the release year, and the title's own
IMDb identity.

**Why this priority**: It is a genuine gap — that source's detail sheet shows no
synopsis today and its IMDb link has to be inferred — and the answer arrives in
the same response this feature already fetches, so it costs nothing extra. P3
because it is not what was asked for.

**Independent Test**: Open a title from that source and confirm the synopsis and
IMDb link are present where previously they were not.

**Acceptance Scenarios**:

1. **Given** a title from the source in question, **When** its detail sheet is
   opened, **Then** the synopsis it publishes is shown.
2. **Given** that source publishes a synopsis in more than one language, **When**
   the synopsis is shown, **Then** one is chosen deterministically and never two
   concatenated.
3. **Given** the source publishes the title's IMDb identity, **When** the sheet is
   opened, **Then** the existing IMDb link uses it.

---

### Edge Cases

- **A person the source names but cannot identify.** A cast entry may carry a
  name with no IMDb identity and no photograph. It is still shown — a name is
  worth more than nothing — as an unlinked initials tile.
- **Two people with the same name.** People are keyed by identity, never by name;
  two different people sharing a name must never collapse into one tile or share a
  cached photograph.
- **A person listed twice on the same title** — a writer who also directed is the
  common case. They appear once under each role, never twice within one, and their
  photograph is resolved once (FR-024).
- **A series with no director.** One source leaves the field empty for most
  series; the creator and writers it does publish carry the section instead
  (FR-004a).
- **A name that is not Latin script.** Names are shown exactly as the source
  publishes them, with the same direction-neutral rendering already used for
  synopses. Initials are derived from the name as published, not transliterated.
- **A source photograph URL that 404s.** Treated as no photograph: the tile falls
  back exactly as if the source had published none.
- **The third-party lookup starts refusing SynoDL** (rate limit, block, redesign).
  Every affected tile degrades to initials; nothing else about Discover changes,
  and the failure is never surfaced as an error to the user.
- **A very large cast.** The number of people shown, and the number of outbound
  lookups one title can cause, are both bounded — a title cannot be a lever for
  hundreds of third-party requests.
- **The same person requested concurrently by several users.** One lookup, not
  one per viewer.
- **A person's photo changes upstream.** Remembered answers expire, so the new one
  is picked up in time without any manual step.
- **A source that publishes cast but the extra lookup fails** (the second source's
  per-person page is down). The cast still renders with names and characters; only
  the faces and links are missing.

## Requirements *(mandatory)*

### Functional Requirements

#### What the sheet shows

- **FR-001**: The Discover title detail sheet MUST show the title's cast and its
  crew — director, creator, and writers — where the source publishes them, in a
  section distinct from the download options.
- **FR-001a**: The people section MUST sit BELOW the download options, so sending a
  download remains the first action the sheet offers.
- **FR-002**: Cast MUST be listed in the billing order the source publishes, not
  re-sorted alphabetically or by whether a photograph was found.
- **FR-003**: Where the source publishes the character a cast member plays, it
  MUST be shown with their name; where it does not, only the name is shown.
- **FR-004**: Each crew role MUST be shown under its own label, separate from the
  cast and from each other, so "who is in it", "who directed it", "whose show it
  is" and "who wrote it" are never merged into one list.
- **FR-004a**: A role the source publishes nothing for MUST be omitted entirely.
  In particular a series whose source leaves the director empty but names a creator
  MUST show the creator, not an empty director line.
- **FR-004b**: Crew roles MUST appear in a fixed order — director, then creator,
  then writers — so the sheet reads the same way on every title.
- **FR-004c**: A person credited in more than one role MUST appear once per role,
  and MUST NOT be repeated within a single role.
- **FR-005**: A title whose source publishes no people at all MUST render no
  people section — no empty heading, no skeleton.
- **FR-006**: Failure to determine who made a title MUST NOT fail the detail
  request: download options, synopsis and existing metadata MUST render exactly as
  they do today.
- **FR-007**: The number of people shown per title MUST be bounded, and the bound
  MUST be large enough to cover a normal billed cast.
- **FR-008**: The people section MUST be built from stock Ionic components and the
  project's existing theme tokens (constitution Principle VI), and MUST render
  correctly in light and dark themes and with right-to-left content.
- **FR-009**: A people list longer than the viewport MUST be browsable without
  causing horizontal scrolling of the sheet or the page.

#### Where the data comes from

- **FR-010**: For the source that exposes an API, cast and crew MUST be read from
  that provider's own title-detail response, and every people field MUST tolerate
  being absent or null without erroring — which is the normal case for a series'
  director on that source.
- **FR-011**: For the source that publishes no API, cast and crew MUST be read
  from the title page the driver already fetches, identified by the page's own
  labels, so no additional request is made to learn the names. A source that
  publishes only some of the roles MUST yield only those.
- **FR-012**: A source that publishes labels in a non-English language MUST be
  matched on those labels as published; unrecognised labelled groups MUST be
  ignored rather than guessed at.
- **FR-013**: A source image that is the provider's own stand-in ("no photo")
  rather than a photograph of the person MUST be recognised as *no photograph* and
  MUST NOT be shown or cached as one.
- **FR-014**: Where a source publishes no IMDb identity for a person on the title
  page but does publish one on that person's own page, SynoDL MAY fetch that page
  to learn it. Those fetches MUST be bounded per title, run within a deadline, and
  MUST degrade to "identity unknown" rather than delaying or failing the sheet.
- **FR-014a**: Those fetches carry the operator's stored source session, and MUST
  therefore be server-initiated only — made while assembling a response SynoDL
  itself decided to assemble. No endpoint may let a client name the person page to
  be fetched. (The IMDb fallback is the opposite case and may be client-triggered
  precisely because it carries no credential and reaches a fixed host.)
- **FR-015**: A person's photograph and IMDb identity, once learned from a
  source's person page, MUST be remembered against the person so no later title
  re-fetches it.

#### The IMDb fallback

- **FR-016**: Where no source photograph exists for a person whose IMDb identity
  is known, SynoDL MUST attempt to obtain their photograph from IMDb, and MUST
  serve it to the client same-origin through SynoDL rather than having the browser
  load a third-party URL.
- **FR-017**: The fallback MUST NOT be performed while assembling the detail
  response: opening a sheet MUST NOT wait on IMDb. Photographs MUST arrive
  independently of the sheet's own render.
- **FR-018**: The outbound surface added by this feature MUST be an explicit,
  fixed allowlist of the IMDb lookup host and the image host it serves
  photographs from. It MUST NOT widen the existing source image proxy's allowlist,
  and no client-supplied host, URL, or path may reach it.
- **FR-018a**: A URL discovered INSIDE third-party content — the image location a
  page names — MUST itself be validated against the same fixed allowlist before it
  is fetched. Being named by an allowlisted host does not make a URL allowlisted.
- **FR-018b**: Redirects MUST NOT be followed off the allowlist: a redirect to a
  host outside it MUST abort the fetch rather than follow it.
- **FR-019**: The identifier a client may use to ask for a person's photograph
  MUST be constrained to the exact shape of an IMDb person identifier, and
  anything else MUST be refused before any outbound request is made.
- **FR-020**: Reading a third-party page for a photograph MUST be bounded in
  bytes and MUST stop as soon as the answer is found, so a large page cannot be
  used to exhaust the server.
- **FR-021**: Outbound fallback lookups MUST be bounded in concurrency
  instance-wide, and the endpoint that triggers them MUST be rate-limited per
  caller, so neither a busy instance nor a hostile one can be used to hammer a
  third party.
- **FR-022**: Where the photograph host supports serving a smaller rendition,
  SynoDL MUST request one sized for a tile rather than the full-size image.
- **FR-023**: A fallback lookup that fails, times out, is refused, or finds no
  photograph MUST result in the person having no photograph — never an error
  reaching the user, and never a failed Discover request.

#### Remembering people

- **FR-024**: Resolved people MUST be cached keyed by the person — their IMDb
  identity, or where that is not yet known, the source's own reference for them —
  never by the title they were seen in.
- **FR-025**: The cache MUST survive a server restart, MUST live in the single
  SQLite store of Principle III, and MUST NOT introduce a second datastore.
- **FR-026**: A "this person has no photograph" answer MUST be cached too, with
  its own shorter lifetime, so a person IMDb has no photo of is not re-fetched on
  every view but is eventually re-checked.
- **FR-027**: Cached answers MUST expire, so an upstream change is picked up
  without operator intervention.
- **FR-028**: Concurrent requests for the same unresolved person MUST result in
  one outbound lookup, not one per request.
- **FR-029**: The cache MUST be bounded — in rows and by expiry — so it cannot
  grow without limit on the operator's volume.
- **FR-030a**: Values taken from a source or a third party — a name, a character,
  an image URL — MUST be length-bounded before they are stored or sent to a client,
  so a hostile or broken upstream cannot put an arbitrarily large value into the
  operator's volume or the user's DOM.
- **FR-030**: Cached rows MUST contain no secret and no per-user data: a person's
  name, their public identity, and a public image URL only. Nothing in it may
  reveal who viewed what.

#### Degradation and safety

- **FR-031**: A person with no photograph from any origin MUST render as a
  readable initials tile in the app's own styling, identical in size and shape to
  a photographed tile.
- **FR-032**: A photograph that fails to load in the browser MUST fall back to the
  initials tile rather than leaving a broken image or a gap.
- **FR-033**: A person tile MUST link to that person's IMDb profile where their
  identity is known, opening the same way the existing IMDb rating link does, and
  MUST NOT present itself as a link where it is not one.
- **FR-034**: Names, characters, and image URLs read from a source or from IMDb
  MUST NOT appear in log lines, error payloads, metrics, or panics, and MUST be
  treated as untrusted content when rendered.
- **FR-035**: Development and the e2e suite MUST be able to exercise every path of
  this feature — source cast, source photo, stand-in detection, fallback hit,
  fallback miss, cache reuse — against the in-repo mocks, with no real source, no
  real IMDb, and no network.

#### Incidental gain

- **FR-036**: Where the newly-fetched provider detail response also carries the
  title's synopsis, release year, and IMDb identity, those MUST populate the
  existing detail fields that source currently returns empty.
- **FR-037**: Where that response publishes a synopsis in more than one language,
  exactly one MUST be chosen by a fixed rule, never concatenated.

### Key Entities

- **Person**: someone who worked on a title, as shown on a tile. Their name as the
  source publishes it, their role in this title (cast, director, creator, or
  writer), the character they play where known, their billing order within that
  role, their public IMDb identity where known, and their photograph where one was
  found.
- **Person photo record**: the remembered answer to "what does this person look
  like": the person's identity, the resolved image location or an explicit "none",
  and when that answer was determined. Keyed by the person, shared by every title
  and every user of the instance.
- **Source person reference**: a source's own handle for a person, used only where
  that source does not publish an IMDb identity alongside the name, and resolved
  once into a person identity.
- **Title detail**: gains its cast and directors, and — for one source — the
  synopsis, year, and IMDb identity it did not previously carry.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Opening a title from either configured source shows who is in it
  and who directed it, without leaving SynoDL.
- **SC-002**: Adding people to the sheet does not make opening a title
  noticeably slower: the sheet appears within the same time budget it does today,
  regardless of how many photographs are still being resolved.
- **SC-003**: For a typical title, at least 80% of the people shown end up with a
  photograph — measured against titles where the sources alone would supply under
  half.
- **SC-004**: A person seen in a second title in the same session costs zero
  additional third-party lookups, and still costs zero after a restart.
- **SC-005**: A person with no photograph anywhere is indistinguishable from a
  design point of view from one with a photograph — same tile size, same
  alignment, no broken images — verified on both themes.
- **SC-006**: With the fallback origin made completely unavailable, every existing
  Discover behaviour — browse, search, filter, open, send — is unchanged, and
  every tile still shows a name.
- **SC-007**: The whole feature is demonstrable and testable with no real source
  credentials, no real third-party access, and no network.
- **SC-008**: No new outbound host beyond the two declared for the photograph
  fallback is reachable, and no client input can steer a request to one.

## Credential-Safety Impact

Required by constitution Principle III. This feature both **widens SynoDL's
outbound surface** and **adds a persisted table**, so both halves are answered.

- **What new hosts are contacted, and why they are bounded.** Two, both
  third-party and both public: one IMDb host, to read a person's page for the
  location of their photograph, and one image host, to fetch that photograph. They
  are a fixed, compiled-in allowlist belonging to this feature alone — deliberately
  *not* added to the existing source image proxy's allowlist, so a widening here
  cannot become a widening there. No client-supplied host, URL, or path reaches
  either: the only client input is a person identifier constrained to the exact
  shape IMDb uses, validated before any outbound request exists (FR-019). This is
  the instinct of the DSM allowlist applied to a third surface — the user picks a
  person within what the source already told us, never a host.
- **What crosses to the NAS.** Nothing. This feature never touches the NAS, the
  DSM allowlist, the stored NAS credentials, or any worker.
- **What is sent to the third party.** A person's public IMDb identifier and
  nothing else: no SynoDL user identity, no session, no referrer tying the lookup
  to a title or a viewer, no cookie. Lookups are made by the server, never by the
  user's browser, so no user's IP address or user agent reaches IMDb — which is
  also why FR-016 requires photographs to be served same-origin rather than
  hotlinked.
- **What is stored, and how it is protected.** One new table of derived public
  facts: a person's identity, their name, a public image URL or an explicit
  "none", and when that was determined. It holds no secret, so it is not encrypted
  under `SECRETS_KEY` — and it must hold nothing that would deserve to be: no user
  id, no title id, no timestamps of who looked at what (FR-030). It is a cache in
  the strict sense — losing it costs lookups, never data — and it is bounded by
  expiry and row count (FR-029) so it cannot grow unbounded on the operator's
  single volume. It lives in that same volume's SQLite database: no second
  datastore (FR-025).
- **Why persisted at all,** given Principle III's preference for deriving.
  Deriving this on every restart means re-scraping a third party for hundreds of
  people, which is both slow for the user and the behaviour most likely to get an
  instance blocked. The record is of a completed lookup, not a mirror of live
  state; nothing about it drifts, and losing it degrades only speed.
- **What could appear in logs or errors.** Names, characters, and image URLs read
  from a source or from IMDb are content, not diagnostics, and MUST NOT reach log
  lines, error payloads, metrics, or panics (FR-034). Failures are swallowed into
  "no photograph" rather than surfaced (FR-023), so a block or an outage cannot
  leak an upstream body or URL into anything a user sees.
- **What a hostile client could do with it.** The photograph endpoint is
  unauthenticated by necessity — an `<img>` tag cannot carry the session header,
  the same constraint the existing poster proxy lives under. It is therefore
  bounded three ways: the identifier shape (FR-019), a per-caller rate limit and an
  instance-wide concurrency cap (FR-021), and a byte bound on what is read from
  upstream (FR-020). A caller cannot use it to reach an arbitrary host, to amplify
  traffic at a third party, or to exhaust the server's memory. What it *can* learn
  — what a public figure looks like — is public.
- **Whose data it is.** The cache is derived from public catalogues and is
  identical for every user of the instance. It is not per-user, reveals nothing
  about any user, and needs no per-user scoping.

## Assumptions

- **Both sources keep publishing this.** Cast and director were verified live on
  both configured sources on 2026-09-16. A source redesign that moves or removes
  them degrades to "no people section" (FR-005) rather than breaking Discover.
- **IMDb identifiers are the right key.** Both sources ultimately identify people
  by IMDb id — one directly, one via the person's own page — so it is the one
  identity both agree on and the natural cache key. A person with no IMDb id is
  simply never cached.
- **The IMDb fallback is best-effort and may be blocked.** Reading a public page
  for an image location is not an API and carries no availability promise. The
  feature is specified so that losing it entirely costs faces and nothing else
  (SC-006).
- **Photographs are public publicity stills.** They are fetched for display beside
  the title they belong to, served same-origin from a per-instance cache,
  not redistributed or reprocessed.
- **A character name is optional data.** Only one source publishes it; its absence
  is normal, not a degraded state.
- **Scope is the Discover title detail sheet.** Not the catalog grid, not search
  by person, not person pages, not filmographies. Filtering by cast and director
  already exists as a search facet and is untouched.
- **Crew is what the source calls it.** Director, creator and writer are taken as
  the source publishes them; SynoDL does not reconcile the two sources' notions of
  a role, infer a creator from a writer, or fill a missing role from the other
  source.
- **Cache lifetimes are operator-invisible.** They are chosen in the plan, not
  configured: a found photograph is remembered far longer than a missing one, and
  neither is an operator-facing knob.

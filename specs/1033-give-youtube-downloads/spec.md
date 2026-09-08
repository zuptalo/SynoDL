# Feature Specification: Give YouTube downloads their own button

**Feature Branch**: `feat/1033-give-youtube-downloads`

**Created**: 2026-09-07

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "do you think the UI/UX when it comes to music video and music from youtube are clear to the end user? I think they are a bit confusing and hard to tell what actually is going to happen given that we have a folder selection at the top as well! maybe we should move the youtube music and music video download to it's own seperate button under the + floating button!"

## Overview

Spec 0012 put the YouTube choice inside the existing new-task sheet, and that
was the wrong home for it.

The sheet opens with **Destination** — a folder chip, a picker, and a "new
folder in movie" action. Choose Music further down and none of that applies any
more, but nothing says so: the picker still reads `movie`, still looks
interactive, and is the *first* thing in the sheet. A person forms a belief
about where their download is going, and only afterwards meets a control that
silently invalidates it. Category and the torrent-file field have the same
problem, less prominently.

Worse, the default is wrong. For a YouTube link the pre-selected option is
Download Station, which cannot extract YouTube at all — it would fetch an HTML
page and call it a download. The one choice that is never right is the one
already selected, and the two that are right sit below the URL box where they
have to be noticed.

This gives YouTube downloads their own button under the existing `+`, opening a
sheet that asks only what it needs: a link, and whether it is music or a music
video. The absence of a folder picker is itself the honest signal — you do not
choose where these go, the mode does, because the libraries are configured once
by the operator.

## User Scenarios & Testing

### User Story 1 - A button that means one thing (Priority: P1)

Someone with a YouTube link taps `+`, picks the music button, pastes the link,
chooses Music or Music video, and taps Add. Nothing on that sheet is about
folders, categories, or torrent files, because none of them apply.

**Why this priority**: It is the fix. Every other part of this spec is about
making sure the old confusing path stops competing with it.

**Independent Test**: Open the new sheet from the FAB and confirm it offers a
link, a mode, and nothing else — then submit a link and see it appear in the
Tasks list.

**Acceptance Scenarios**:

1. **Given** the server can run YouTube downloads, **When** the user opens the
   `+` menu, **Then** a dedicated action for saving from YouTube is offered
   alongside the existing URL and file actions.
2. **Given** the user opens that sheet, **When** it renders, **Then** it shows a
   link field and a Music / Music video choice, and shows no destination
   picker, no category, and no file field.
3. **Given** a YouTube link and a chosen mode, **When** the user submits,
   **Then** the download is accepted and appears in the Tasks list.
4. **Given** a link that is not a supported YouTube address, **When** the user
   submits, **Then** the sheet says so plainly and nothing is started.
5. **Given** the server cannot run YouTube downloads, **When** the user opens
   the `+` menu, **Then** the action is not offered at all, rather than
   offered and then failing.

---

### User Story 2 - The old sheet stops being misleading (Priority: P1)

The general new-task sheet no longer offers a YouTube mode. Pasting a YouTube
link there instead points the user at the button that handles it properly.

**Why this priority**: Leaving both paths in place would keep the confusing one
alive and add a second way to do the same thing. Removing the mode choice from
the sheet is what makes the new button the answer rather than an alternative.

**Independent Test**: Paste a YouTube link into the general sheet and confirm no
Send-to control appears, and that the sheet instead points at the dedicated
button.

**Acceptance Scenarios**:

1. **Given** a YouTube link pasted into the general new-task sheet, **When** it
   is detected, **Then** no mode selector appears and the destination controls
   keep meaning exactly what they say.
2. **Given** that same link, **When** the sheet notices it, **Then** it tells the
   user there is a better way to save it to a media library.
3. **Given** a non-YouTube link, **When** the user uses the general sheet,
   **Then** nothing about that flow has changed.

---

### Edge Cases

- **A mix of links pasted at once.** The dedicated sheet is for YouTube; a paste
  containing both YouTube and non-YouTube links must not silently send the
  wrong ones to the wrong place.
- **Several YouTube links at once.** Submitting more than one must start each,
  and must report clearly if some started and one failed.
- **A link that is already downloading.** The user must be told, rather than
  seeing a generic failure.
- **The feature turned off mid-session.** An operator who removes the
  configuration must not leave a button that opens a sheet that cannot work.
- **A very long link.** The field must not push the submit control off-screen.

## Requirements

### Functional Requirements

- **FR-001**: Users MUST be able to reach YouTube downloading from a dedicated
  action in the `+` menu on the Tasks view.
- **FR-002**: That action MUST NOT be offered when the server cannot run YouTube
  downloads.
- **FR-003**: The dedicated sheet MUST ask only for a link and a mode, and MUST
  NOT present destination, category, or file controls.
- **FR-004**: The dedicated sheet MUST require an explicit choice between Music
  and Music video, and MUST NOT default to a mode that discards the user's
  intent.
- **FR-005**: The general new-task sheet MUST NOT offer a YouTube mode selector.
- **FR-006**: When a YouTube link is present in the general sheet, System MUST
  tell the user that a dedicated way to save it exists.
- **FR-007**: Submitting an unsupported link MUST report why, in plain language,
  and MUST start nothing.
- **FR-008**: Submitting several links MUST attempt each, and MUST report how
  many were started when not all succeed.
- **FR-009**: Behaviour of the general sheet for non-YouTube links MUST be
  unchanged.

### Key Entities

- **Download request** — a link plus a mode. Unchanged from spec 0012; this
  feature changes only where the user expresses it.

## Success Criteria

- **SC-001**: A user saving a song touches no control that does not apply to
  saving a song.
- **SC-002**: No path in the app lets a YouTube link be sent to Download
  Station by default.
- **SC-003**: A user who pastes a YouTube link into the general sheet is told
  where it should go instead.
- **SC-004**: The `+` menu offers the action only where it can work.

## Credential-Safety Impact

- **Nothing new is stored, sent, or logged.** This moves an existing control to
  a different sheet. The endpoint, its host allowlist, and its argv handling are
  unchanged from spec 0012.
- **The allowlist still decides.** The dedicated sheet does not relax what counts
  as an acceptable link; validation stays server-side where it was.

## Assumptions

- **The readable-row work is separate.** Making a YouTube row show a real title
  and thumbnail instead of a URL needs the server to fetch metadata at submit
  time — a new outbound surface that deserves its own spec and its own
  Credential-Safety section. This spec is only about where the user expresses
  the request.
- **One sheet, not a wizard.** A link and a two-way choice fit on one sheet;
  splitting them across steps would be worse.
- **The existing FAB list is the right home.** It already offers two actions, so
  a third needs no new pattern.

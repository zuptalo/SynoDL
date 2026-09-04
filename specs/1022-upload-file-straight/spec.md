# Feature Specification: Upload a file straight into your library

**Feature Branch**: `feat/1022-upload-file-straight`

**Created**: 2026-09-03

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "in the tasks section also give the user to upload a file directly in
the target folder they want to add a file to, like directly from their phone or pc, it still
should only allow them to do it in the movie or tv-show folder only and they should be required to
provide a plex friendly name for the title they are uploading, we should also consider the scenario
where they might be adding a new episode to an existing tv-show."

## Overview

Everything SynoDL puts on the NAS today arrives by telling Download Station to fetch a URL. There
is no way to put a file there that the user already has — an episode recorded elsewhere, a
subtitle file, a poster for a title the scraper cannot match.

This adds a direct upload from the device the user is holding, into the same movies and TV parent
folders SynoDL already writes to, under the same naming convention (spec 1021) and the same
per-user folder permissions. The library stays tidy because the name is not optional: the user
states the title, and SynoDL builds the folder.

**This crosses a boundary SynoDL has never crossed.** Until now the proxy could create an empty
folder on the NAS but could not write a single byte of content. Uploading requires a new DSM API
and makes SynoDL able to place arbitrary file content on the operator's NAS. That is the whole
reason this needs a spec and a credential-safety checklist rather than a patch.

## Clarifications

### Session 2026-09-03

- Q: How large may an upload be, given every byte streams from the device through the SynoDL
  container to the NAS? → A: Episodes and sidecars, with a hard cap around 2 GB. A single streamed
  request per file, never buffered, with progress and cancel. Larger media stays a File Station or
  SMB job; a file over the cap is refused with a clear message rather than half-transferred
  (FR-020, FR-021).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Add a movie you already have (Priority: P1)

A user holding a film file picks it, types the title, and it lands in the movies parent under a
correctly named folder — the same name a download of that title would have produced.

**Independent Test**: Upload a file with the title "Dune 2021"; it appears at
`movie/Dune (2021)/<file>`.

**Acceptance Scenarios**:

1. **Given** a chosen file and the title "Dune 2021", **When** the user uploads, **Then** the file
   lands in `movie/Dune (2021)` — the year moved into parentheses, exactly as a send would name it.
2. **Given** a title already in the correct shape ("Dune (2021)"), **When** the user uploads,
   **Then** the folder is not double-parenthesised.
3. **Given** the folder already exists, **When** the user uploads, **Then** the file is added to
   it rather than a second folder being created.
4. **Given** no title is entered, **When** the user tries to upload, **Then** it is refused — the
   name is required, which is what keeps the library tidy.

### User Story 2 - Add a new episode to a show you already have (Priority: P1)

A user with `tv-show/Friends (1994)` already on the NAS uploads one episode. They pick the
existing show rather than retyping it, choose the season, and the file lands in the right place.

**Independent Test**: With `tv-show/Friends (1994)/Season 01` present, upload an episode into
season 1 and find it there, with no duplicate show folder created.

**Acceptance Scenarios**:

1. **Given** shows already on the NAS, **When** the user starts a TV upload, **Then** they can
   pick an existing show instead of typing a name.
2. **Given** an existing show is picked, **When** the file uploads, **Then** it lands under that
   exact folder — no near-duplicate is created for a differently-typed name.
3. **Given** a season is chosen, **When** the file uploads, **Then** it lands in `Season NN`.
4. **Given** the chosen season folder does not exist yet, **When** the file uploads, **Then** it
   is created.
5. **Given** no season is chosen, **When** the file uploads, **Then** it lands directly in the
   show's folder, which a scraper still reads.

### User Story 3 - Add a new show, or a sidecar (Priority: P2)

**Acceptance Scenarios**:

1. **Given** a show not yet on the NAS, **When** the user types its title and a season, **Then**
   both folders are created and the file lands inside.
2. **Given** a title folder that exists, **When** the user uploads a subtitle or artwork file,
   **Then** it lands beside the video rather than in a folder of its own.
3. **Given** several files chosen at once, **When** the user uploads, **Then** each is placed in
   the same destination, and one failing does not abandon the rest.

### Edge Cases

- A file whose name collides with one already in the destination.
- A file larger than the cap.
- The connection dropping part-way through.
- A title that reduces to nothing usable ("...", "///").
- A user with no folder grant on the chosen parent.
- No download source configured, so no parent folders are known.
- A device that offers no MIME type, or a wrong one.
- Two uploads to the same new folder at once.

## Requirements *(mandatory)*

### Functional Requirements

**Where a file may go**

- **FR-001**: An upload MUST be allowed only into the movies or TV parent folders SynoDL is
  configured to use. No other path may be targeted, however the request is formed.
- **FR-002**: The destination MUST be derived by the server from the parent, the title, and the
  optional season. A client MUST NOT be able to supply a path directly.
- **FR-003**: Per-user folder permissions MUST be enforced on the final destination, exactly as
  they are for a download.
- **FR-004**: Where no parent folders are configured, upload MUST be refused with a clear reason
  rather than defaulting to somewhere.

**Naming**

- **FR-005**: A title MUST be required; an upload with no title MUST be refused.
- **FR-006**: The title MUST be converted to the same convention a download uses — `Title (Year)`
  — so an uploaded title and a downloaded one are indistinguishable afterwards.
- **FR-007**: A title already in that shape MUST NOT be altered.
- **FR-008**: A title that reduces to nothing usable MUST be refused, not silently replaced.
- **FR-009**: A season, when given, MUST produce a `Season NN` subfolder.
- **FR-010**: Choosing an existing show MUST use that folder exactly, so no near-duplicate is
  created.

**Placing the file**

- **FR-011**: Missing folders in the destination MUST be created; existing ones MUST be reused.
- **FR-012**: An upload MUST NOT overwrite a file already at the destination. The attempt MUST be
  reported, not silently dropped and not silently overwritten.
- **FR-013**: The original file name MUST be preserved — only the folder is SynoDL's to choose.
- **FR-013a**: The file name MUST be a single path segment. A name containing a path separator, a
  parent-directory reference, or a control character MUST be REJECTED rather than repaired, so a
  client-supplied name can never extend the path the server composed.
- **FR-013b**: Only media and sidecar files MUST be accepted — video, subtitle, artwork, and
  metadata types. Anything else MUST be refused, so the capability granted stays the size of the
  request rather than becoming a general "write any file to the NAS" tool.
- **FR-014**: Several files MUST be uploadable in one action, each reported on its own, with one
  failure not abandoning the others.

**Moving the bytes**

- **FR-015**: File content MUST stream from the request to the NAS. It MUST NOT be buffered whole
  in memory, nor written to disk on the server, which persists nothing.
- **FR-016**: Progress MUST be visible while a file uploads.
- **FR-017**: An upload MUST be cancellable, and cancelling MUST stop the transfer.
- **FR-018**: A failure MUST say which file failed and why, in words.
- **FR-019**: A dropped connection MUST NOT leave the server holding resources.

**Limits**

- **FR-020**: An upload larger than the configured cap MUST be refused before the transfer starts
  where the size is known, and MUST be stopped if the cap is exceeded mid-stream.
- **FR-021**: The cap MUST be stated in the interface, so a user is not surprised by a refusal.
- **FR-021a**: Uploads MUST be rate-bounded per user, so a signed-in client cannot use the
  endpoint to fill the operator's volume as fast as the link allows.

**Boundaries**

- **FR-022**: File names MUST NOT appear in logs, error payloads, metrics, or panics.
- **FR-022a**: An upload MUST require an authenticated session. There is no anonymous path to it.
- **FR-023**: An upload MUST NOT create a download task, appear in download statistics, or count
  against a user's download allowance. It is not a download.

## Success Criteria *(mandatory)*

- **SC-001**: A film on a phone reaches the NAS, correctly named, without touching File Station.
- **SC-002**: An episode joins an existing show without creating a second folder for that show.
- **SC-003**: An uploaded title is indistinguishable from a downloaded one — same naming, matched
  by Discover's "already have it" marker.
- **SC-004**: An upload cannot place a file outside the configured movies and TV parents, under
  any request a client can make.
- **SC-005**: An oversized file is refused with a clear message and no partial file is left behind.
- **SC-006**: The container's memory does not grow with the size of the file being uploaded.

## Credential-Safety Impact

Required by constitution Principle III.

- **What is being added to the DSM allowlist, and why it is the largest widening so far.** One
  API: the FileStation upload endpoint. Until now the proxy could enumerate folders and create an
  empty one; it could not write content. After this it can place arbitrary bytes on the
  operator's NAS. That is a genuine increase in what a compromised or buggy SynoDL could do, and
  it is the reason this is a spec rather than a patch. It is justified by being the only way to
  satisfy the request at all: the client cannot reach DSM directly and must never hold NAS
  credentials.
- **Two client-supplied strings reach the path, and both are constrained.** The title is
  sanitised into a single segment; the FILE NAME is validated as one and refused if it is not
  (FR-013a). The second is the easier to overlook, because "preserve the original name" reads as
  a courtesy rather than as accepting untrusted input into a path. Accepted file types are limited
  to media and sidecars (FR-013b), which keeps the granted capability the size of the request
  instead of a general write-anything tool, and uploads are rate-bounded per user (FR-021a).
- **What bounds it.** The destination is never client-supplied. The server composes it from a
  parent it already knows (the configured movies/TV folders), a title run through the same
  single-segment sanitiser used for downloads, and an optional season it formats itself. Every
  segment is validated to reject separators and traversal, and the finished path is checked
  against the user's folder grants — the same check a download passes. There is no request shape
  that reaches a path outside those parents.
- **What is stored.** Nothing. Bytes stream through and are not buffered whole in memory nor
  written to the server's volume, so an upload leaves no trace on the server — consistent with
  the store holding no file content of any kind.
- **What could appear in logs.** File names are user content and MUST NOT reach log lines, error
  payloads, metrics, or panics. Errors returned to the client name the file the client already
  knows about; the server does not log it.
- **Not a download.** Uploads deliberately do not touch the daily download allowance, the
  statistics history, or task ownership. Conflating them would let an upload distort accounting
  that exists to protect the operator's download quota.

## Assumptions

- The device's browser provides the file; SynoDL never reads the device's filesystem itself.
- Around 2 GB covers episodes and sidecars, which is the stated need. Larger media remains a File
  Station or SMB job, and the interface says so rather than failing mysteriously.
- The parent folders come from the configured download sources, the same set Discover's ownership
  scan reads, so upload and download always agree on where a library lives.

## Out of Scope

- Resumable or chunked uploads. A dropped transfer is retried from the start.
- Uploading anywhere other than the movies and TV parents.
- Renaming or transcoding the uploaded file.
- Editing or deleting files already on the NAS.

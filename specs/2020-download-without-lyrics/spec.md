# Feature Specification: A download without lyrics is not a failure

**Feature Branch**: `fix/2020-download-without-lyrics`

**Created**: 2026-09-07

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Observed in production: a track downloaded, tagged and saved correctly, then reported as failed. Worker log ends `[Exec] Executing command: … ERROR: Postprocessing: Command returned error code 1`, after `[info] There are no subtitles for the requested languages`.

## Overview

A download that worked is being reported as failed.

The worker fetches the audio, writes it to the library, embeds the metadata and
the cover art — and then the last step, a one-line rename that tidies the lyrics
file next to it, exits non-zero when there is no lyrics file to tidy. The
downloader treats that as post-processing failure, exits 1, and the Job is
marked failed. The user sees **failed** next to a track that is sitting in their
library, correctly named and correctly tagged.

The rename loop ends with a test for a file that is not there. In a shell, an
unmatched glob leaves the pattern as literal text, the existence test is false,
and because that test is the last command in the loop, its false result becomes
the exit status of the whole command.

Reproduced exactly:

```
exit code with NO lyrics file present: 1
exit code WITH a lyrics file present:  0
```

The same flaw is in the music-video path, where the equivalent line ends with a
comparison that is false when no subtitle file matched.

This is worth more than its size. Spec 0012 went to some trouble to ensure a
failed download is never reported as a success, on the grounds that a silent
failure is the worst outcome. This is the same error inverted, and it is nearly
as bad: it teaches the user to distrust a status that is otherwise reliable, and
it hides a real failure among false ones.

## User Scenarios & Testing

### User Story 1 - A saved track reads as saved (Priority: P1)

Someone saves a track that has no lyrics published — an instrumental, a live
set, an ambience recording. It downloads, it lands in the library correctly, and
the app says so.

**Why this priority**: It is the bug. Every download of a track without lyrics
is currently mislabelled, and lyrics are absent more often than present.

**Independent Test**: Run the tidy-up step in a directory holding only an audio
file and confirm it succeeds.

**Acceptance Scenarios**:

1. **Given** a download whose item publishes no lyrics, **When** it finishes,
   **Then** it is reported as completed.
2. **Given** a download whose item does publish lyrics, **When** it finishes,
   **Then** the lyrics file is still renamed to sit beside the audio file, and
   it is reported as completed.
3. **Given** a music-video download with no subtitles, **When** it finishes,
   **Then** it is reported as completed.
4. **Given** the tidy-up cannot rename a file for some other reason, **When** it
   finishes, **Then** the download is still reported as completed, because the
   media file is written and a companion file is not worth failing over.

---

### Edge Cases

- **Several companion files.** More than one matching file must still be handled
  without the step failing.
- **A genuine download failure.** Fixing this must not make a real failure look
  like a success — only the tidy-up step becomes non-failing, not the download.

## Requirements

### Functional Requirements

- **FR-001**: The companion-file tidy-up MUST NOT cause a download to be
  reported as failed, under any circumstances.
- **FR-002**: When a companion file exists, it MUST still be renamed to sit
  beside the media file.
- **FR-003**: FR-001 MUST hold for both the music and music-video paths.
- **FR-004**: A download that genuinely fails MUST still be reported as failed;
  this change MUST NOT make failures invisible.

## Success Criteria

- **SC-001**: A track with no lyrics reports as completed.
- **SC-002**: A track with lyrics reports as completed and has its lyrics file
  correctly named beside it.
- **SC-003**: A download that actually fails still reports as failed.

## Credential-Safety Impact

None. This changes the exit status of a shell snippet that contains no user
input and touches no credential, no stored data, and no network.

## Assumptions

- **A companion file is not worth a failure.** The media file is the download.
  If tidying its sidecar fails, the right outcome is a saved track with a
  slightly awkwardly named companion file, not a download reported as lost.
- **The existing rename behaviour is correct** and only its exit status is
  wrong; this spec does not revisit the naming rules, which media servers
  depend on.

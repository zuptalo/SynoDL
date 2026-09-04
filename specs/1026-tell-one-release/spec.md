# Feature Specification: Tell one release from another by the file it makes

**Feature Branch**: `feat/1026-tell-one-release`

**Created**: 2026-09-04

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "yes, parse the zarfilm series encoders too"

## Context

Spec 1025 identifies which copy of a title is on the NAS by comparing two tokens
from the file name: the resolution and the release group. That works where the
group actually names who encoded the file. Checked against the live source, it
does not work for ZarFilm at all:

- Every file it serves is renamed with the site's own suffix, so a file encoded
  by NHTFS, by PSA, by Pahe and by RMTeam all report the **same** group. The
  token that was supposed to tell them apart tells them apart from nothing.
- Some of its files carry that suffix without a separator, and identify nothing
  at all.
- For a movie the site labels **every** option with its own name as the encoder,
  so the options are not distinguishable by group even in principle.
- Its series options carry no encoder at all today, so nothing about them can be
  matched and nothing is ever marked.

The result is that ZarFilm titles are either never marked, or — for the one shape
that does parse — marked too broadly, several options at once, which is the bug
spec 1025 set out to fix.

There is something that does tell these releases apart: the file each option
produces. The download link names it, and it is the name the file lands under, so
the option and the file on disk can be compared directly instead of through
tokens that the site has overwritten.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which version you have, on every source (Priority: P1)

A user with ZarFilm configured opens a title they have already downloaded. Today
nothing is marked, so the screen is no help at all in telling which of six near
identical options they already took.

After this change the option that produced the file on disk is marked, and only
that one — including for the two options that differ by nothing a token can see,
such as a dubbed encode and a subtitled one at the same resolution.

**Why this priority**: This is the case the previous spec could not reach, and it
is the majority of one source's catalog.

**Independent Test**: Put a known ZarFilm release on the NAS, open the title, and
confirm exactly the option that produced it is marked.

**Acceptance Scenarios**:

1. **Given** a file on the NAS produced by one option, **When** the title opens,
   **Then** that option is marked.
2. **Given** two options at the same resolution whose only difference is the
   encode, **When** one of them is on disk, **Then** the other is not marked.
3. **Given** a season on disk from which some episodes are missing, **When** the
   title opens, **Then** the option that produced the episodes present is still
   marked — a partial season identifies its release just as well as a full one.
4. **Given** a file on the NAS that no option would have produced, **Then** no
   option is marked and the title is still reported as present.
5. **Given** a source whose options do not name the file they produce, **Then**
   matching behaves exactly as it does today.

### User Story 2 - See who encoded a season (Priority: P2)

A series' options show a size and an episode count but not who encoded them, so
two rows at the same resolution read identically even though one is a much better
encode.

**Independent Test**: Open a ZarFilm series; each season option names its encoder
alongside the size, as a movie's options already do.

**Acceptance Scenarios**:

1. **Given** a season option whose quality names an encoder, **When** the option
   is shown, **Then** the encoder is displayed as it is for a movie.
2. **Given** a quality that names no encoder, **Then** the option shows what it
   does today, with nothing invented.

### Edge Cases

- Two options that would produce the same file must not cause one to be marked
  and the other not; if they are genuinely indistinguishable, both are.
- A file whose name matches an option except for its episode number must still
  identify that option.
- A link that names no file (a paywall placeholder) must not be treated as a
  release, and must not match a file with no name either.
- A source that renames files on the way in — so what lands does not match what
  was asked for — must degrade to marking nothing rather than marking wrongly.
- The file name must not appear in the API response, in a log, or in an error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A download option MUST be able to carry the name of the file it
  would produce, supplied by the source driver.
- **FR-002**: That name MUST NOT be sent to the client, logged, or included in any
  error or metric.
- **FR-003**: Two releases MUST be judged the same when the files they name are
  the same, disregarding case, separators and the episode number.
- **FR-004**: Where an option names the file it produces, that comparison MUST
  decide the outcome: a mismatch means the option is NOT the one on disk, and MUST
  NOT fall back to comparing tokens.
- **FR-005**: Where an option does not name a file, matching MUST fall back to the
  existing resolution-and-group comparison.
- **FR-006**: Failure to identify a release MUST NOT change whether the title or
  season is reported as present.
- **FR-007**: The ZarFilm source MUST supply the file name for movie options and
  for season options.
- **FR-008**: The ZarFilm source MUST read the encoder from a season's quality
  description and expose it as the option's encoder.
- **FR-009**: A quality description naming no encoder MUST leave the option's
  encoder empty rather than guessing.
- **FR-010**: 30nama's behaviour MUST be unchanged unless it also names the files
  its options produce.

### Key Entities

- **Release identity** — what makes two copies the same release: the file they
  are named as, reduced so that the episode number and formatting differences do
  not matter.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a ZarFilm title with a known release on the NAS, exactly the
  option that produced it is marked.
- **SC-002**: Options that differ only by an encode a token cannot see are told
  apart.
- **SC-003**: A partially downloaded season still identifies its release.
- **SC-004**: No file name appears in any response, log or error.
- **SC-005**: Sources that do not name their files behave exactly as today.
- **SC-006**: A ZarFilm season option shows its encoder.

## Assumptions

- The file lands under the name the download link gives it. That is how the NAS
  names what it fetches, and it is why the two sides can be compared at all. Where
  it does not hold, the comparison simply fails to match and nothing is marked —
  which is the conservative outcome already required.
- Comparing the file an option produces is strictly better evidence than comparing
  tokens taken from it, because the source rewrites those tokens and does not
  rewrite the fact that this link yields this file.
- 30nama continues to use the token comparison until it, too, names the files its
  options produce.

## Credential-Safety Impact

- No change to the DSM API allowlist and no additional NAS call: the comparison
  uses file listings already fetched for season presence.
- The file name an option would produce is source catalog data, not NAS content.
  It is held only for the life of the request and is explicitly excluded from the
  API response by its wire definition, so it cannot be serialised by accident.
- NAS file names continue never to leave the server and never to enter a log. The
  reduced identity derived from them stays in the same in-memory cache as the
  season presence beside it and is rebuilt on restart.
- The client's view is unchanged: it still learns only whether an option is one it
  already has.

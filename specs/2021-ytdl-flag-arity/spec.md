# Feature Specification: Progress reporting and the traversal guard, actually switched on

**Feature Branch**: `fix/2021-ytdl-flag-arity`

**Created**: 2026-09-09

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Observed on the live deployment: a submitted playlist showed its title
and artwork and then did nothing. Investigation found progress reporting had
never worked in 0.16.0, and the path-traversal guard shipped with it was inert.

## Overview

Spec 0013 shipped two things that turned out not to be running at all: the
progress reporting that is the whole point of the feature, and the
`--replace-in-metadata` guard that closes a path traversal in the download
recipe.

Both were disabled by one mistake. `--replace-in-metadata` takes **three**
arguments — `FIELDS REGEX REPLACE`, `nargs=3` — and they were passed as one
space-joined string. Each occurrence of the option therefore consumed the two
argv elements that followed it. With three guards in a row, that ate `--newline`
and `--progress-template`, and left the progress template itself to be parsed as
a positional URL:

```
[generic] Extracting URL: [synodl] status=%(progress.status)s …
ERROR: [generic] '[synodl] status=%(progress.status)s …' is not a valid URL
```

Nothing failed loudly. The real URL was still the last argv element, so downloads
still ran and still saved correctly — they simply never reported progress, and
the guard never applied.

The tests did not catch it because they asserted the option was PRESENT and that
its value contained the right pattern, which a joined string satisfies. The
verification of the guard was done through the extractor's Python API rather than
its command line, which is precisely where the arity lives.

## User Scenarios & Testing

### User Story 1 - A download reports progress again (Priority: P1)

Someone starts a download and watches the bar move, as spec 0013 promised.

**Why this priority**: it is the headline of the release it regressed in.

**Independent Test**: run the worker with the argv the server produces and
confirm the sentinel progress lines appear and no stray URL is parsed.

**Acceptance Scenarios**:

1. **Given** the argv the server builds, **When** the worker runs, **Then** it
   emits the progress lines SynoDL asked for.
2. **Given** the same argv, **When** it is parsed, **Then** the submitted link is
   the ONLY positional argument.

---

### User Story 2 - The traversal guard is actually applied (Priority: P1)

A source that names its artist or album with nothing but dots cannot place files
outside the media library.

**Why this priority**: it is the security fix spec 0013 added, and it has not
been in force since it shipped.

**Independent Test**: parse the server's argv with the extractor's own option
parser and confirm three `(field, regex, replace)` actions are registered.

**Acceptance Scenarios**:

1. **Given** the argv the server builds, **When** it is parsed, **Then** each
   guarded field appears as a separate three-argument action.

---

### Edge Cases

- **An option whose arity changes upstream.** The failure was silent because a
  wrong-arity flag consumes its neighbours instead of erroring. Any future flag
  taking more than one argument carries the same risk.
- **A stray positional.** Anything that is neither a flag nor a flag's argument
  reaches the extractor as a URL, which is how this surfaced.

## Requirements

### Functional Requirements

- **FR-001**: Every worker option MUST receive its arguments as separate argv
  elements, in the number the option actually takes.
- **FR-002**: The submitted link MUST be the only positional argument in the
  worker's argv.
- **FR-003**: A download MUST emit the progress lines SynoDL specified, in the
  format SynoDL defined.
- **FR-004**: The dot-only guard MUST be registered for every field that becomes
  a directory component.
- **FR-005**: These properties MUST be verified against the pinned worker image's
  own command-line parser, not only against its Python API — the arity of an
  option exists only at the command line.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Running the server's exact argv against the pinned image produces
  SynoDL's progress lines and no "is not a valid URL" error.
- **SC-002**: Parsing that argv yields exactly one positional URL and three
  metadata-replace actions.
- **SC-003**: A regression test fails if any option is given the wrong number of
  arguments.

## Credential-Safety Impact

- **A security fix was inert.** The path-traversal guard from spec 0013 (FR-038a)
  has not been in effect in any shipped build. The exposure it covers is
  unchanged from before spec 0013 — a source-supplied artist or album consisting
  only of dots could write outside the intended library — and is closed by this
  fix rather than newly introduced by it.
- **No new values reach a worker**, no new permission, no change to what is
  stored or logged.

## Assumptions

- The pinned worker image is unchanged (`jauderho/yt-dlp:2026.08.19`).
- No data written by an affected build needs correcting: downloads completed
  normally and were filed correctly; only the reporting and the guard were off.

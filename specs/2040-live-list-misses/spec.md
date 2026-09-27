# Feature Specification: The live Tasks list no longer misses a download that finished just as you opened it

**Feature Branch**: `fix/2040-live-list-misses`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Found while upgrading the toolchain: the YouTube e2e suite failed intermittently with a row stuck at "Starting" after its job had failed, while the API already said "failed".

## Overview

The live update stream (spec 1038) publishes what changed between two
reconcile cycles by fingerprinting every unfinished download at the end of
each cycle. To cost nothing while nobody was watching, it skipped that work —
including the fingerprinting — whenever there were no subscribers.

That left a gap. A download that was unfinished during a cycle nobody watched
had no fingerprint. If somebody then opened the Tasks page and the download
finished before the NEXT cycle, that cycle found it neither among the
unfinished (it was done) nor among the fingerprints (never taken) — so it
had nothing to say, and said nothing. The page kept showing the state from
its initial read, "Starting", for as long as the stream stayed healthy, which
is indefinitely. The window is a few seconds after opening the page, which is
exactly when the e2e suite drives a job to fail, and exactly when a person
who has just added a download is looking.

## Requirements

- **FR-001**: The fingerprints MUST be kept current on every cycle, whether or
  not anybody is subscribed, so a change that straddles the first watched cycle
  is announced. Publishing still happens only when there is somebody to tell.
- **FR-002**: The first announcement after the process starts still carries the
  unfinished set as CHANGED rows, never as created (spec 1038 FR-004a).
- **FR-003**: The regression MUST be covered by a unit test: unfinished with
  nobody watching → a subscriber → finished → the subscriber hears it.

## Out of scope

- The client's own small race (a list snapshot answered after a newer frame
  overwrites it). Different mechanism; not what was observed.

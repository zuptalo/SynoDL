# Feature Specification: A YouTube download that YouTube turned away tries again by itself

**Feature Branch**: `feat/1043-retry-refused-youtube`

**Created**: 2026-09-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "go for it please" — on the proposal to re-queue refused downloads automatically after a cool-down, following spec 2035.

## Overview

Measured in production on 2026-09-27 during a bulk run of several thousand
tracks: the first ~850 downloads after 0.18.3 had no refusals at all; after
about 50 minutes of sustained downloading YouTube began turning requests away —
raw worker errors `HTTP Error 403: Forbidden` on the media itself and `Sign in
to confirm you're not a bot`. The rate barely moved when parallelism went from 4
to 2 (4.6% → 3.9%): it is the volume from one address that YouTube reacts to.
Every refused track re-probed later downloaded fine.

So these are "not now", not "no" — and asking a person to come back and tap
retry on each is the manual routine the feature exists to remove.

### Narrowing 0012 FR-021 / 0013 FR-027

Both say nothing retries automatically, "so that a partially completed run is
never repeated on its own". That was written when a channel was ONE job. Since
spec 0013 every track is its own download, and a refused one saved nothing, so
retrying it repeats no completed work. This spec narrows the rule rather than
dropping it: ONLY a refusal is retried by itself, a bounded number of times,
after a growing wait. Every other failure still needs a person.

## Requirements

- **FR-001**: A download that failed because YouTube turned it away (HTTP
  403/429, bot check — as classified by spec 2035) MUST go back to the queue by
  itself once `YTDL_AUTO_RETRY_AFTER_SECONDS` × (attempts so far) has passed
  since it failed, until it has had `YTDL_AUTO_RETRY_MAX_ATTEMPTS` attempts in
  total. Defaults 1800 s and 3. `1` switches automatic retry off.
- **FR-002**: A failure for any other reason — gone, private, paid, region,
  age-restricted, unrecognised — MUST NOT be retried automatically.
- **FR-003**: While it waits, the download MUST say it will try again by itself;
  after its last attempt it MUST read as the plain refusal, with Retry offered as
  for any failure.
- **FR-004**: A retried download re-joins the BACK of the queue and remains one
  download with one more attempt (0013 FR-029).
- **FR-005**: A playlist with a track awaiting an automatic retry has not
  finished: it MUST read as downloading and MUST NOT notify until that track
  settles. A single download awaiting a retry MUST NOT notify either.
- **FR-006**: Refusals recorded before this feature (reason "turned away",
  attempts to spare) MUST be picked up as well.

## Out of scope

- Signing in to YouTube (cookies) for age-restricted or members-only videos.
- Adaptive throttling (slowing admission when refusals appear). The cool-down
  plus the queue already spreads retries out; this can follow if refusals
  persist.

# Feature Specification: YouTube downloads stop waiting forever once a few hundred have run

**Feature Branch**: `fix/2034-download-queue-stalls`

**Created**: 2026-09-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "please have a look at the home k3s and status of our pod and see why we have many downloads still unfinished"

## Overview

On the production cluster, 4,557 downloads sat `queued` for ~21 hours with no
worker running, four items stayed `scheduled` although their workers had
finished, and the server logged nothing at all.

**Cause.** Every reconciler cycle lists the Jobs this feature owns. Finished
Jobs linger for `YTDL_TTL_SECONDS` (24h). A Job weighs ~6 KiB, and after a bulk
channel expansion 700 of them had accumulated: the list came to **4,204,671
bytes**, just over the k8s client's 4 MiB read cap (`internal/k8s/jobs.go`). The
body was cut off, failed to decode (`unexpected end of JSON input`), and the
reconciler — which deliberately does not log a list failure per cycle — returned
before recording outcomes or admitting anything. No new Job could be created, so
the list never shrank below the cap until the TTL swept Jobs a day later, at
which point it would have refilled within hours: throughput was capped at
roughly 700 downloads per TTL window.

The pods list had outgrown the cap as well (5.1 MB). That silently stopped
progress reading and, through `podLogFor`, would have failed any finished
playlist/channel expansion as "could not read what that link contains" —
permanently.

**Immediate remediation (operational, done 2026-09-27).** The 696 finished Jobs
whose outcome was already in the store were deleted; the 4 whose outcome was not
were left for the reconciler, which recovered on its next cycle and resumed
admitting.

## Requirements

- **FR-001**: Listing Jobs and pods MUST return the complete set however many
  exist, by paging (`limit` + `continue`) with the label selector on every page.
  A page that cannot be fetched fails the whole list — a partial list would read
  as vanished workers.
- **FR-002**: A response exceeding the read cap MUST surface as a named error
  (`ErrResponseTooLarge`), not as a JSON decode failure.
- **FR-003**: A download's Job MUST be deleted once — and only once — its final
  outcome is recorded in the store. Until then the Job is the only evidence of
  how the download ended and MUST be kept. `YTDL_TTL_SECONDS` remains as a
  backstop for Jobs the server never reached. This is consistent with spec 0013,
  which makes the record, not the Job, what history and the list are built from.
- **FR-004**: Pod lookups (a running worker's progress, a finished enumeration's
  output) MUST be narrowed to the request they are for, so their size does not
  grow with the number of finished workers.
- **FR-005**: The reconciler MUST log once when it starts failing to list
  workers (WARN, with the error) and once when it recovers (INFO) — never per
  cycle.
- **FR-006**: Sweeping a dismissed download's Job MUST happen only when the
  store says the record does not exist, not on any read error.

## Success criteria

- **SC-001**: A Jobs list of any size is read whole (regression: 900 × 6 KiB).
- **SC-002**: In production, after deploy, `queued` falls steadily and the
  number of lingering finished Jobs stays near zero rather than growing toward
  the cap.

## Out of scope

- The per-video failures seen among workers (age-gated, unavailable, HTTP 403)
  are yt-dlp/YouTube outcomes, recorded correctly, and unrelated.

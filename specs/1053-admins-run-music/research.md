# Research: Admins run the music library repair from Settings

Findings come from reading the server (`internal/k8s`, `internal/ytdl`, `internal/api`
reconciler, store migrations, router), the client (Settings, `MusicLibraryModal`,
`api.ts`), the Dockerfile and the mock cluster, and from running the tool for real in
spec 1052. Nothing here was guessed about how the cluster behaves that the download
workers do not already demonstrate.

## R1 — How does the worker get code that is exactly the server's version?

**Decision**: an **init container** in the worker pod runs the SynoDL image the server
is itself running, by digest, and copies `/opt/music_repair` into an `emptyDir`; the
main container (the pinned worker image, which already has Python, mutagen and ffmpeg)
runs `python3 -m music_repair` with `PYTHONPATH=/code`. The Dockerfile's runtime stage
gains `COPY scripts/music_repair/ /opt/music_repair/`.

**Why**: FR-018 (same version) is then true by construction — the bytes the worker runs
are the bytes in the server image that launched it, so an upgrade cannot leave the two
apart. No duplication of the Python in the Go tree, no generated copy to keep in sync,
no ConfigMap (which would need `create configmaps` RBAC — a widening this spec forbids).

**How the server knows its own image**: it reads its own pod (`GET pods/$HOSTNAME`,
already permitted by `pods: get`) and takes the running container's `imageID`
(`repo@sha256:…`), falling back to the container's `image`. `MUSIC_REPAIR_IMAGE`
overrides it (dev and e2e, where there is no such pod). If neither is known the feature
is *unavailable, with the reason*, never half-working.

**Alternatives**:
- *Embed the Python in the Go binary (`go:embed`) and pass it in the Job spec.* Rejected:
  `go:embed` cannot reach `scripts/` from the `server/` module, so it forces either a
  checked-in generated copy (drift, a CI guard, 2,900 duplicated lines) or moving the
  tool's sources and rewiring the operator wrapper, CI and docs. Larger and worse.
- *ConfigMap of the code.* Rejected: needs a new RBAC verb.
- *A new worker image.* Rejected: a second image build/publish pipeline and a second
  thing to keep pinned, for the sake of files that already ship in the server image.

**Constitution reading (flagged for review)**: "worker images are pinned third-party
tags, never `:latest`". The init container is SynoDL's own image, pinned by digest —
stricter than a tag — and it executes only `cp`. The image that does the actual work is
the pinned worker image, unchanged.

## R2 — How does the server learn what a run is doing without reading the share?

**Decision**: the tool prints **event lines** — `@@synodl {json}` — alongside its human
output. The server reads the pod's log through the permission it already has, keeps only
lines with that prefix, and decodes them into a fixed Go type.

Two events: `progress` (`phase`, `done`, `total`) and `result` (`kind`, `ok`, `reason`,
`planId`, `summary`). The full schema is `contracts/worker-events.md`.

**Bounds (constitution v2.2.0: worker output is user data)**: the log read is capped at
256 KiB by the existing client and tailed to the last 200 lines; an event line over
its cap (4 KiB progress, 16 KiB result) is dropped; at most 64 event lines are considered; every integer is clamped to
`[0, 10¹⁵]`; every string is stripped of control characters and length-capped; unknown
fields and unknown events are discarded; at most 20 example paths and 12 reasons are
kept. Nothing from the log is written to the server's own log, and nothing but the
decoded, re-serialised shape reaches a client.

**A missing or malformed report** degrades to *finished, no summary available* (for a
Job that completed) or *did not finish* (for one that did not) — never a guess.

## R3 — State: what is stored, what is derived

**Stored** (constitution Principle III, "the request, and its finished outcome"): one
row per run in `music_repairs`: kind, plan id, who, times, state and the bounded summary.
It is written when the run starts (state `running`) and updated once, when the run is
seen to end.

**Derived**: whether a run is *running* right now comes from the Job (`status.active`,
conditions), and its progress from the log. A row that says `running` for a Job that is
gone or failed is corrected to `unfinished` — the row never overrides the cluster.

**Capturing the outcome before the Job is swept**: done on the existing download
reconciler's tick (CLAUDE.md: "ONE reconciler loop … so they cannot race over the same
Jobs list"), as one more step. The Job TTL is 24 h, so there is ample margin; the tick
is seconds.

## R4 — "Only one at a time", including races and the command line

Layered, because each layer covers a different failure:

1. **Database**: a partial unique index on `state = 'running'`. Inserting the row *is*
   acquiring the slot, atomically; two simultaneous requests cannot both succeed (SC-003).
2. **Cluster**: before creating the Job, list Jobs labelled `app.kubernetes.io/name=synodl-music-repair`
   — which also matches a Job started by `scripts/music-repair.sh` — and refuse if any is
   still active, saying it is running from the command line.
3. **The tool's own lock** on the share (`.repair/lock`), which it already refuses on
   and reports as `reason: locked` in its result event.

If creating the Job fails after the row was inserted, the row is marked `unfinished`
with a "could not start" summary so the slot is not held forever.

## R5 — What may be applied, and when

A plan is applicable iff its **check** run finished with `ok`, produced a `planId`, is
younger than 24 h (clarified; counted from its finish), and no **apply** run for that
plan has finished. An apply that ended `unfinished` can be **continued**: the same
request again, which the tool resumes from its journal (FR-010). Undo applies to the most
recent apply that started (finished or not — the tool restores whatever its journal
recorded) and has not been undone.

The server holds only the plan id and the summary. It never sees the plan, so it cannot
apply a plan it did not make — and says so (spec edge case). The apply request carries
`planId` and `snapshotAck: true`; the server refuses a request without either (FR-008,
clarified) rather than trusting the client to have enforced it.

**Plan id safety**: a plan id reaches the Job only as one discrete argv element, and only
after matching `^\d{8}T\d{6}Z-[0-9a-f]{6}$` — the constitution's argv rule, made structural.

## R6 — The Job

Same shape as a download worker, with two differences: an init container (R1) and a much
longer deadline (12 h; a first lookup pass is 2–3 h at the sources' rate limit).
`restartPolicy: Never`, `backoffLimit: 0`, `automountServiceAccountToken: false`,
`runAsUser/Group` from the existing download-worker setting, exactly ONE library (the
music claim), an `emptyDir` for `/code` and `/tmp`. Labels `managed-by=synodl`,
`kind=music-repair` (so the download reconciler, whose selector is `kind=ytdl`, never
sees it and its orphan sweep never touches it), `repair-id=<id>`, and the shared
`app.kubernetes.io/name=synodl-music-repair` of R4.

## R7 — Client

Stock Ionic in the Settings pattern: an admin `ion-list` entry opens `MusicRepairModal`
(as `MusicLibraryModal` does). Live state is a 3-second poll of one endpoint while the
modal is open and a run is active — not a new stream: a repair is one run, seen by one
admin at a time, and the download stream's machinery would be disproportionate. The pure
part (what to offer, how to word progress and expiry, byte formatting) is a module with
unit tests (`repair-state.ts`) so the component stays thin and the coverage floor holds.

The apply confirmation is a second modal with an `ion-checkbox` ("I have taken a snapshot
of the music share") that gates an `ion-button`; the server independently refuses a
request without `snapshotAck` (clarified).

## R8 — Testing without a real cluster

The mock Kubernetes API already provides Jobs, pod logs, `POST /__mock/jobs/{name}/emit`
(append log lines) and succeed/fail/deadline/vanish. That is sufficient: an e2e test
starts a check, emits `@@synodl` lines and succeeds the Job, and asserts the screen.
`MUSIC_REPAIR_IMAGE` stands in for the self-image lookup in dev and e2e; the lookup
itself is unit-tested against an `httptest` fake. A real check is run on the cluster
after deploy (the tool is already proven there; what is new is the launch and the report).

## Open items carried to tasks (none block)

- The tool must print the events; the current human lines stay (operators read them).
- Whether the download reconciler's tick has a natural hook for the capture step is
  confirmed while implementing; if it does not, the step is called from the same
  goroutine that runs the tick — never from a second loop.

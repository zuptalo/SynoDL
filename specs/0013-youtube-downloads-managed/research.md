# Research: YouTube downloads you can watch, keep, and retry (spec 0013)

**Phase 0 output** · Resolves every unknown in the plan's Technical Context.

Each finding is marked with its confidence. **Verified** means checked against
this repository. **Established** means it follows from documented behaviour of a
dependency we already ship. **To verify** means it must be confirmed against real
content or the pinned worker image before the task that depends on it is called
done — these are called out again in `quickstart.md`.

---

## R1 — How the server reads a worker's output

**Decision**: List the worker's pod by the Job's own label selector (already
possible), then read that pod's log through the orchestrator's `pods/log`
subresource. Add `pods/log` with verb `get` to the existing namespaced Role.

**Rationale**: The RBAC already grants `pods: get, list` — spec 0012 added it so a
Job's state could be reported accurately. Finding the worker pod therefore needs
no new permission at all; only reading its output does. `pods/log` is a
subresource with its own RBAC entry, so granting it grants exactly reading
output and nothing else. It remains a namespaced Role. It does not imply
`pods/exec`, `pods/attach`, or `secrets`.

**Wire shape** (Established): `GET /api/v1/namespaces/{ns}/pods/{pod}/log`, with
`container`, `tailLines`, `sinceSeconds`, `timestamps`. Returns `text/plain`, not
JSON — the existing `Client.do` decodes JSON and cannot be reused as-is; a
sibling method that returns bytes is needed.

**Alternatives considered**:

- *Worker calls back into SynoDL.* Rejected: needs our own image wrapping the
  pinned upstream extractor (against the constitution's "worker images are pinned
  and deliberately updated"), a per-job credential inside the pod, and a network
  path from worker to server.
- *A shared volume the worker writes progress to.* Rejected: the server must not
  mount a media volume (Principle III), so this needs a *third* volume existing
  only to carry progress.
- *Watch the Jobs API.* Rejected: Job status has no progress in it at all.

**Consequences for `internal/k8s`**: the package doc says "enough to create,
list, and delete Jobs in ONE namespace, and nothing else". That is now three
calls plus two: list pods by selector, and read one pod's log. The doc comment
must be updated in the same change, not left to drift.

---

## R2 — Making the worker's progress a format we define

**Decision**: Run the worker with `--newline` and an explicit
`--progress-template` carrying a fixed sentinel prefix, so SynoDL parses a line
it specified rather than human-readable text that changes between releases.

**Rationale** (Established): yt-dlp's default progress output rewrites one line
with carriage returns and is meant for a terminal; parsing it is exactly the
brittle scraping the spec set out to avoid. `--progress-template` renders fields
the caller names, and `--newline` makes each update its own line. The template is
a constant authored in our source, contains no user input, and is testable in
isolation — the same property `execRenameLyrics` already relies on.

**Fields available to the template** (Established): `progress.status`,
`progress.downloaded_bytes`, `progress.total_bytes`,
`progress.total_bytes_estimate`, plus `info.*` for the item being fetched,
including `info.id`, `info.playlist_index` and `info.n_entries`.

**A sentinel prefix is load-bearing.** The worker's stdout also carries ordinary
yt-dlp chatter. A line SynoDL acts on must be one SynoDL asked for, so the
template starts with a fixed marker and anything without it is ignored rather
than guessed at.

**To verify**: exact field availability for the audio path, where the download
completes before extraction and the post-processing steps produce no progress
events of their own.

---

## R3 — Two passes, one progress bar

**Decision**: Report progress as *completed fraction of the item*, computed from
which pass is running rather than from bytes alone; never let the displayed value
decrease.

**Rationale** (Established): the music-video format selector
(`bv*[vcodec^=avc1]+ba[acodec^=mp4a]/bv*+ba/b`) deliberately asks for separate
picture and sound streams, because YouTube's only progressive format is 640x360
and is served intermittently. Separate streams mean yt-dlp reports two
independent runs of 0→100%. Rendering those raw would show a bar that restarts,
which FR-012 forbids.

`info.n_entries` / `info.playlist_index` are NOT the answer here — after
expansion every worker fetches exactly one item.

**Approach**: treat the fragment/stream index the template exposes as a phase,
map each phase onto a sub-range of the whole, and clamp monotonically in the
server so a late-arriving stale line cannot move the bar backwards. Clamping
server-side rather than client-side matters: FR-013f means several clients read
the same held value, so the value itself must already be correct.

---

## R4 — Expanding a playlist or channel

**Decision**: A dedicated short-lived expansion Job runs the extractor in
listing-only mode and prints one line per entry; the server reads that from the
Job's pod log (R1) and creates one download record per entry.

**Rationale** (Established): `--flat-playlist` lists a playlist or channel
without resolving each entry, which is what makes enumerating a large channel
cheap. Pairing it with `--print` and an explicit template gives one line per
entry in a format we define, consistent with R2. `-J` (single JSON document) was
rejected: a channel of thousands produces one enormous document, whereas
line-per-entry streams and bounds naturally.

**Why not in the server process**: the server has no extractor binary, and
Principle III / 0012 FR-005 keep this work out of the server. Adding an extractor
to the server image would also make the server image no longer just a Go binary.

**Why not the site's official data API**: it needs an API key — a new credential
under Principle III custody, plus a quota and a second failure mode — to obtain
something the extractor already returns.

**Expansion output is untrusted** (FR-013h, FR-016a): each entry yields a URL
SynoDL then hands to another worker. Every one goes back through the SAME host
allowlist as a user-typed link. Spec 0012's `Classify` is reused unchanged; the
new thing is that it is now also applied to links SynoDL derived rather than only
to links a user submitted.

**Scale**: with no ceiling (FR-017), one channel can produce thousands of
records. Consequences: entries are inserted in one transaction, the list is
paged (FR-006a), and the expansion job's own log read is bounded (FR-013e).

---

## R5 — Album naming survives expansion

**Decision**: Pass the group's name to each item's worker as an explicit
metadata argument, and keep the folder template and the tag template the single
shared expression they already are.

**Rationale** (Verified, `server/internal/ytdl/command.go`): `tmplArtist` is
`%(artist,uploader)s`, so artist ALREADY falls back to the channel name and
FR-035 is satisfied today. `tmplAlbum` is `%(album|Singles)s` and does not fall
back to a playlist name, so FR-036 needs a change.

**The interaction that matters**: an expanded item is fetched as a single video
with `--no-playlist`, so `playlist_title` is simply not populated — the obvious
fix of `%(album,playlist_title|Singles)s` silently does nothing after expansion.
The group's name must therefore be supplied by the server, which knows it from
R4, as a discrete argument (FR-038).

**The identity between folder and tag is load-bearing** and must be preserved:
`command.go` splices the same constants into `-o` and into `--parse-metadata`
precisely so a track cannot be foldered as one thing and tagged as another.

---

## R6 — A source-derived name must not escape the library

**Decision**: Sanitise the group name server-side before it ever becomes an
argument — reject or strip path separators and parent-directory references, and
bound its length (FR-038a, FR-038b) — and do not rely on the extractor's own
filename sanitising to do it.

**Rationale**: this is the checklist's highest-risk finding (CHK019). FR-038's
discrete-argument rule prevents a *command* being injected and does nothing about
a *path*, and the same value becomes a directory name inside the mounted library.
The extractor does sanitise output-template field values (Established: it
replaces path separators with lookalike characters), which means the likely
outcome today is a mangled folder name rather than an escape — but "the
dependency probably handles it" is not the standard for the one thing that would
put files outside the library a request was aimed at.

**Defence in depth, deliberately**: sanitise in the server (ours, tested,
table-driven) AND keep the worker's own sanitising. Neither alone is the
argument.

**To verify**: that the extractor's sanitising applies to a value injected via
metadata parsing, not only to values it extracted itself.

---

## R7 — Where the queue lives and who admits from it

**Decision**: One reconciler ticker in the server process does everything
periodic: list this feature's Jobs, read running workers' output, capture
terminal facts durably, and admit from the queue up to the limit.

**Rationale** (Verified): the repo already has this shape — `library_scan.go` and
`source_keepalive.go` are `time.NewTicker` loops started from
`cmd/synodl/main.go`. One loop rather than three avoids three tickers racing over
the same Jobs list, and makes "read output because a download is running, not
because someone is looking" (FR-013f) structural rather than a rule to remember.

**Single admitter** (Verified, `deploy/k8s/10-synodl.yaml:106`): `replicas: 1`.
Admission needs no distributed lock. The plan does NOT add one, and records that
a multi-replica deployment would need one — the spec puts it out of scope.

**Fair-share** (FR-022b, FR-022c): admission picks by rotating over users with
queued work, and within one user prefers a directly-submitted download over an
expanded item. Both are pure functions over the queued set, so they are unit
testable without a cluster — which is where this logic belongs, following
`task-sort.ts` and `internal/ytdl`'s existing table-tested style.

---

## R8 — Delivering progress to the client

**Decision**: The reconciler holds the latest progress in memory; `GET /v1/ytdl`
serves it. No new stream, and no progress in the database.

**Rationale**: FR-013f requires reads not to scale with viewers, which rules out
reading a worker's output inside a request handler. Holding the value in memory
and serving it from the existing poll keeps the client unchanged in shape (it
already polls every 5s, Verified `src/composables/useYtdl.ts`).

Holding progress in memory is explicitly NOT a mirror of orchestrator state under
Principle III: it is a cache of a reading, it is lost on restart without
consequence, and the durable record holds only the request and its finished
outcome (FR-013g).

**Alternative considered**: extend the existing SSE stream (Verified,
`internal/api/tasks_stream.go`, 1s poll + 15s heartbeat). Rejected for this spec:
that stream is built around the NAS sid and per-user task snapshots, and
YouTube downloads change state far more slowly than a NAS transfer. A 5s poll is
adequate for a progress bar. Worth revisiting only if it proves not to be.

---

## R9 — Cover art on the music-video path

**Decision**: Treat embedded cover art for mp4 as UNPROVEN and satisfy FR-034
with an accompanying artwork file if embedding cannot be confirmed against the
pinned image.

**Rationale** (Verified): `command.go` passes `--embed-thumbnail
--convert-thumbnails jpg` for both modes. For mp3 this is known to work. For mp4
the extractor needs a helper tool present in the image, and when it is absent it
warns and continues — so the download succeeds and the art is silently missing,
which is precisely the failure mode nobody notices.

Pinned image (Verified, `deploy/k8s/10-synodl.yaml:60`):
`jauderho/yt-dlp:2026.08.19`.

**To verify — this is a real experiment, not a code read**: run the video path
against the pinned image and inspect the output file. If art is embedded and the
media server displays it, FR-034 is met as-is. If not, write a sidecar. The spec
was written to accept either outcome so this cannot block.

---

## R10 — Storage shape and the migration

**Decision**: One new table holding every download, with a self-referencing
parent for group membership. Migrate the existing `ytdl_failures` rows into it
and leave that table in place, unread.

**Rationale** (Verified, `server/internal/store/schema.go`): migrations are an
append-only list, and the comment is explicit — *never edit a shipped migration,
append a new one*. It is equally explicit that a migration must be re-runnable,
because the schema-drift repair (spec 1031) rewinds `schema_migrations` and
replays: "A migration that cannot run twice turns that repair into a boot
failure." Every statement therefore uses `IF NOT EXISTS`, and the backfill must
be idempotent (`INSERT OR IGNORE`).

**Dropping `ytdl_failures` is deliberately NOT done**: dropping a table is not
re-runnable in the useful direction and buys nothing.

**Deleted users** (Verified): the existing table uses
`user_id INTEGER REFERENCES users(id) ON DELETE SET NULL`, with a comment that
deleting an account must not erase the record, "matching how tasks and downloads
already outlive the account that started them". **This contradicted FR-006d as
first written**, which said records are removed with the user. The spec was
corrected to match the codebase rather than the codebase to match the spec: the
convention is established, documented, and right.

---

## R11 — Where the detail view goes

**Decision**: A modal, matching `TaskDetailModal.vue`. No new route.

**Rationale** (Verified): `src/components/TaskDetailModal.vue` and
`e2e/task-detail.spec.ts` establish the pattern — a task row opens a stock-Ionic
detail sheet with `data-testid="task-detail"` and `detail-*` field ids. The
router has no per-item routes at all; every route is a tab or a gate
(`src/router/index.ts`). Introducing the first detail *route* for this feature
would make YouTube downloads behave unlike every other row in the same list.

The group view (FR-019a) is the one genuinely new surface: a list of items inside
a group. It follows the same modal convention.

---

## R12 — Dates and times

**Decision**: Format explicitly, not by device locale.

**Rationale** (Verified): the only date helper is `src/utils/format.ts`, which
uses `toLocaleString(undefined, …)` — device locale. FR-031 requires a fixed
format on every device, so this feature must not reuse that path. `sv-SE` happens
to render `2026-11-21` and 24-hour times, but relying on a locale to produce a
format is relying on locale data; the format is a product decision (Assumptions),
so it is written out.

---

## R13 — Keeping dev and e2e off a cluster

**Decision**: Extend `internal/k8smock` with pod listing, pod logs, and
`/__mock/*` controls to emit progress lines, lyrics lines, and expansion entries.

**Rationale** (Verified): the mock's own package doc states why it exists —
faking only at the Go interface boundary "would leave the wire format, the label
selector, and the lifecycle-to-state mapping untested — which is precisely where
this feature's bugs would live". Progress parsing, log reading and expansion
output are the same kind of surface, so they belong in the mock too.

Two e2e stacks already exist (Verified, `e2e/stateful/`), and YouTube downloads
are already covered there (`ytdl.spec.ts`, `ytdl-fab.spec.ts`) — so this feature
is e2e-testable without new harness work beyond the mock's new controls.

---

## Open items carried into implementation

| Ref | To verify | Blocks |
|---|---|---|
| R2 | Progress fields on the audio path, where post-processing follows the download | Progress for Music mode |
| R6 | That the extractor sanitises a value injected via metadata parsing | Nothing — server-side sanitising is required regardless |
| R9 | Whether mp4 cover art embeds against the pinned image | Choice between embedded art and a sidecar; FR-034 accepts either |

None of the three blocks the plan. Each is an experiment with a defined fallback
already written into the spec.

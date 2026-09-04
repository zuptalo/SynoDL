# Research: evidence-based ownership, seasons and episodes

Phase 0 for [plan.md](./plan.md). Each entry records what was chosen, why, and what
was rejected.

## 1. What counts as a video file

**Decision.** Reuse the video extension set already in `server/internal/library`
(`upload.go`'s `uploadExt`), extracted into a shared `IsVideo(name string) bool`.
Subtitle, artwork and `.nfo` extensions in that same map are explicitly *not* evidence.

**Rationale.** The set already exists, already governs what may be uploaded, and is
already tested. One definition of "video" for both writing and reading means an
uploaded file is recognised as owned by the same rule that let it in.

**Alternatives considered.** Probing file size (a 0-byte file is not content) — rejected:
`SYNO.FileStation.List` returns size only with an `additional` parameter, and a partially
downloaded file is legitimately non-zero and still incomplete. FR-001b handles the
in-progress case by task state instead, which is exact rather than inferred.

## 2. Both on-disk layouts, from one listing

**Decision.** For a title folder, request `filetype=all` once: directories are candidate
season folders, files are candidate episodes. Seasons come from directory names where
present, and from `SxxEyy` in file names when episodes sit directly in the title folder.

**Rationale.** FR-015 requires both layouts. One listing answers both, so the flat layout
costs one call and the nested layout costs one call plus one per season actually present.

**Alternatives considered.** Two calls (`filetype=dir` then `filetype=file`) — rejected as
double the round trips for the same information.

## 3. Episode numbers

**Decision.** Reuse `seasonEpisode` in `server/internal/library/plexname.go`, which already
matches `S01E02`, `s1.e2` and `1x05`, bounded on non-alphanumerics rather than `\b` because
sources separate with underscores. Extract the episode number alongside the season, which
the current regex captures but discards.

**Rationale.** It is already the twin of `SE_RE` in `src/services/task-title.ts` and is
already covered by the shared corpus. A second parser would drift from it.

**Alternatives considered.** Parsing episode ranges (`E01-E03` in one file) — deferred:
FR-016b already says unreadable numbers must not prevent the season being reported present,
so a missed range degrades to "season present, that episode unlisted" rather than a wrong
answer.

## 4. Paying for verification

**Decision.** Two-layer index. Layer one is today's per-parent directory listing, unchanged,
answering "is there a folder that could be this title" for free. Layer two verifies a
*specific* folder holds video, cached per folder path with the same 5-minute TTL, and is
consulted only for titles being returned in the current response.

**Rationale.** Most catalog items on any page match no folder at all and terminate at layer
one. Only matches — typically 1–5 per page — cost a listing. This satisfies FR-010b
directly and keeps SC-009 ("no slower than before") achievable.

**Alternatives considered.** `SYNO.FileStation.Search` would answer recursively in one call
per parent, but it is **not on the allowlist**, and adding a `SYNO.*` API is a spec-level
decision under Principle III. Rejected as out of scope; noted as the obvious optimisation
if the per-folder cost ever becomes a problem. A full eager scan was rejected under
Complexity Tracking in the plan.

## 5. Distinguishing downloading from owned

**Decision.** Derive it from the task list the server already polls, matching a task's
destination against the title's folder path. No NAS call, no new state.

**Rationale.** FR-001b requires the distinction and explicitly forbids extra NAS reads. The
server already fetches tasks for the Tasks tab, and `source_downloads` already records the
destination each send was given, so both sides of the match are in hand.

**Alternatives considered.** Treating any recent send as "downloading" — rejected: a task
can fail or be removed, and the marker would then be stuck on a title nothing is fetching.
The live task list self-corrects.

## 6. synomock must model a real file tree

**Decision.** Replace the flat `uploads map[string][]string` with a per-directory file map
mirroring the existing `folders` map, and honour `filetype` (`dir` / `file` / `all`) in the
list handler. Seed fixtures through the existing `POST /__mock/library` control endpoint.

**Rationale.** This is a precondition, not a nicety. Two bugs shipped in 0.3.0 — an
unregistered API and a misplaced session id — were invisible because the mock accepted what
DSM refuses. A flat map cannot express `Show/Season 01/ep.mkv`, so every season test would
assert a shape the NAS never returns.

**Alternatives considered.** Testing the listing shape only in `syno` and using fakes above
it — rejected: the fake `syno.Client` in `api/` is where the index logic is tested, and it
would inherit whatever shape the mock legitimised.

## 7. Where ownership is computed

**Decision.** Server-side, decorating catalog responses, exactly as `InLibrary` is decorated
today. The client renders state it is given and computes nothing.

**Rationale.** Principle IV keeps client-side state to app data; ownership is derived from
the NAS and must not be cached in IndexedDB where it would outlive its 5-minute validity.
FR-010c also requires "no marker until verified", which is simplest to honour where the
verification happens.

**Alternatives considered.** A client-side ownership cache keyed by title — rejected: it
would need its own invalidation on send, on source change, and on parent change (FR-008,
FR-008a), duplicating logic the server already owns.

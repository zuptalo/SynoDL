# Tasks: Repair the music library

**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: REQUIRED (constitution II). They run in the pinned image (`docker run --rm -v "$PWD/scripts:/s" -w /s --entrypoint python3 jauderho/yt-dlp:2026.08.19 -m unittest discover -s music_repair -p 'test_*.py'`) with `MUSIC_REPAIR_REQUIRE_DEPS=1`, so a missing mutagen/ffmpeg FAILS instead of skipping. Within every phase the test tasks come first and MUST fail before the
implementation tasks that satisfy them. Test runner:
`python3 -m unittest discover -s scripts/music_repair -p 'test_*.py'`.

**Issues**: #304 Setup and gate · #305 Foundational · #306 US1 · #307 US2 · #308 US3 · #309 US4 · #310 US5 · #311 Delivery (the PR into `main` lists `Closes #304` … `Closes #311`).

**Format**: `- [x] T### [P?] [US?] description with file path` — `[P]` = different files, no dependency on an
incomplete task.

## Phase 0: Gate before code

- [x] T000 Run `/speckit-checklist` (security + allowlist focus) for this spec and resolve every gap it reports in spec/plan/tasks (required by CLAUDE.md for specs on the credential/allowlist boundary); done — `checklists/security.md`, 32 items, all resolved; then `/speckit-taskstoissues`

## Phase 1: Setup

- [x] T001 Create the package skeleton `scripts/music_repair/` (`__init__.py`, `__main__.py` stub with `plan|apply|restore` subcommands parsing the flags in `contracts/cli.md`) and a `VERSION` constant used in the User-Agent
- [x] T002 [P] Build the fixture-library generator `scripts/music_repair/fixtures.py`: creates a tmp library in the OLD layout (`Artist/<playlist>/<title>.mp3` with real tiny mp3s via ffmpeg + mutagen tags `purl`, `comment`, `title`, `artist`, `album`; `.lrc`, `.nfo`, `folder.jpg`, one `.webm`, one `.bin`, a colon-named folder, a `Singles` folder, `.ytdlp-archive.txt`); skips only when `MUSIC_REPAIR_REQUIRE_DEPS` is unset and `ffmpeg`/`mutagen` are missing, and FAILS when it is set

## Phase 2: Foundational (blocks every story)

- [x] T003 [P] Write failing tests `scripts/music_repair/test_names.py`: title/artist cleaning table (the real inputs in research R6, incl. fullwidth quotes, `feat.` lifting, `(Live)`/`(Remix)` kept, "VEVO" stripped from folder names, idempotence `clean(clean(x)) == clean(x)`), and sanitising table (`: * ? " < > |`, dot-only, trailing dots, 120-char bound, NFC) — FR-008, FR-010; channel-marker vocabulary is an EXACT whole-token suffix list only (`VEVO`, `- Topic`, `Official`, `Official Channel`, `Official YouTube Channel`) so `42 dugg Music` and `Music Brokers` are untouched; the cases live in `scripts/music_repair/names_cases.json` so spec 1053 can reuse them against Go's `sanitize.go` (U1, D1)
- [x] T004 Implement `scripts/music_repair/names.py` (`clean_title`, `split_artist_title`, `clean_artist_folder`, `sanitize_name`, `safe_rel`) until T003 passes
- [x] T005 [P] Write failing tests `scripts/music_repair/test_inventory.py` against the fixture library: reads `video_id` from `purl` then `comment`, `duration_s`, `size`, `mtime_ns`, `folder_artist/folder_playlist`, `settled` flag, lyrics sidecar pairing, non-mp3 files classified (`webm`, `bin`, `nfo`, `image`, `archive`), colon-folder detection
- [x] T006 Implement `scripts/music_repair/inventory.py` (`scan(library) -> Inventory`, mutagen ID3 read with no audio decode, excludes `.repair/`, `.trash/`, `Playlists/`) until T005 passes
- [x] T007 [P] Write failing tests `scripts/music_repair/test_sources.py` (allowlist part; also the cover is verified as an image by magic bytes and size-bounded, FR-015, SC-009): suffix match on a dot boundary (`evilarchive.org` rejected), HTTPS only, every redirect hop re-checked, max 3 hops, body cap, timeout, 1 req/s spacing for MusicBrainz, User-Agent set, no query string or env value ever appears in log output (FR-013, FR-015, FR-020)
- [x] T008 Implement the guarded fetcher in `scripts/music_repair/sources.py` (`ALLOWED_HOSTS`, `guarded_fetch(url, fetcher=…)`, `RateLimiter`, injectable transport so tests never touch the network) until T007 passes
- [x] T009 [P] Write failing tests `scripts/music_repair/test_matching.py`: exact artist+title after normalisation and `|delta| <= 3 s` is confident; 3.0 s passes, 3.1 s fails; near-miss title fails; `feat.` normalised both sides; lead artist = first credit; album choice per research R5 (Album/EP/Single with no secondary types, earliest date; compilation-only → album `null` but still confident) — FR-011
- [x] T010 Implement `scripts/music_repair/matching.py` (`is_confident`, `choose_release`, `Match` dataclass) until T009 passes

**Checkpoint**: names, inventory, guarded fetch and the confidence rule work in isolation.

## Phase 3: User Story 1 — See exactly what would change before anything does (P1) 🎯 MVP

**Goal**: `plan` reads the library and writes `plan-<id>.json` + `.md`; nothing else changes.
**Independent test**: run `plan` on the fixture library, hash the tree before/after (identical apart from `.repair/`), read the plan.

- [x] T011 [US1] Write failing tests `scripts/music_repair/test_planner.py::PlanIsReadOnly` and `test_report.py`: tree snapshot identical after `plan`; plan has totals, reasons, `library_fingerprint`; `--repair-dir` outside the library leaves the library byte-identical; an interrupted plan leaves no partial `plan-*.json` (write temp + rename) — FR-001, FR-002, SC-003
- [x] T012 [US1] Implement `scripts/music_repair/planner.py` skeleton (`build_plan(inventory, matches) -> Plan`, action ids, ordering per `contracts/plan-file.md`) and `scripts/music_repair/report.py` (`plan.md` renderer: totals table, per-kind counts, conflicts, unidentified, no-match list, samples; `bytes_needed` vs free space with the shortfall stated when insufficient) and files whose mtime is newer than the scan start are listed as `skip: changed during run` (C5)
- [x] T013 [US1] Wire `plan` in `scripts/music_repair/__main__.py`: scan → (lookup unless `--no-lookup`) → build → atomic write of json + md into the repair dir; print id + totals; exit codes per contract; write `.repair/results-<id>.json` (counts by kind, skipped + why) and progress lines (FR-020, C3)
- [x] T014 [US1] Write failing `test_cli.py`: `plan` twice yields two ids and the same action set; `--limit` respected; `--repair-dir` respected

**Checkpoint**: MVP — an operator can audit the library safely.

## Phase 4: User Story 2 — One copy of every song, playlists kept (P1)

**Goal**: duplicates by video id become `trash` actions; playlists become `.m3u8` files; lyrics follow the kept copy.
**Independent test**: fixture with one video id in three playlist folders across two artists → one file kept, two `trash` actions, and one merged playlist per distinct title.

- [x] T015 [P] [US2] Write failing `scripts/music_repair/test_identity.py`: group by id; kept copy = largest size → earliest mtime → path (each tie level); no-id file kept and listed `unidentified`; settled files are never re-grouped as playlist sources; same-title different-id songs both kept — FR-004, FR-005
- [x] T016 [P] [US2] Write failing `scripts/music_repair/test_playlists.py`: title-only identity across artists, case/whitespace-insensitive merge, ordering artist→title, relative paths that resolve from `Playlists/`, `#EXTM3U` header + UTF-8, merge into an existing file (union, no duplicates), `Singles` folders are not playlists, an artist literally named "Playlists" is renamed `Playlists (artist)` and reported — FR-006
- [x] T017 [US2] Implement `scripts/music_repair/identity.py` and `scripts/music_repair/playlists.py` until T015/T016 pass
- [x] T018 [US2] Extend `planner.py`: emit `keep`/`trash`/`playlist` actions and carry each `.lrc` sidecar with its kept audio (trash the sidecars of trashed copies only if the kept copy has one, otherwise move the best sidecar across); add tests in `test_planner.py` for lyrics-follow (FR-005) and totals (`duplicates_to_trash`, `bytes_reclaimed`)
- [x] T019 [US2] Write failing `scripts/music_repair/test_applier.py::TestTrashAndPlaylists`: playlist files contain no control characters/newlines from titles (FR-006); trash moves preserve the original relpath under `.trash/<id>/`, nothing is deleted, playlist files are written/merged, kept file untouched, `restore` puts trashed files back AND reapplies previous tag values recorded in the journal, skipping (and reporting) an entry whose original location is occupied, and never emptying `.trash` — FR-007, SC-010
- [x] T020 [US2] Implement `scripts/music_repair/applier.py` core (`apply(plan, library)`: safe_rel check on every path, staleness guard `(size, mtime_ns)` checked ONCE per source file at its first action (later retag/move/cover on the same file follow it via the journal; test: one file retagged and moved in one apply is NOT stale), journal `.repair/journal-<id>.jsonl`, resume by skipping `done`, never overwrite an existing target) with `trash` and `playlist` kinds, plus `restore`; until T019 passes
- [x] T020a [US2] Write failing tests for FR-021/FR-022 in `test_applier.py`: a plan edited to contain an absolute path, `..`, or a symlink component is rejected before anything moves; the scanner does not follow symlinks; a second concurrent run refuses on the `.repair/lock`, a stale lock is reported (with pid/host/time) and not taken over; then implement in `applier.py`/`inventory.py`
- [x] T021 [US2] Write failing tests for apply safety in `test_applier.py`: `apply` without `--plan` exits 2; refuses when `plan-<id>.json` is missing or the id unknown; a source changed since planning is skipped and reported; a second `apply` of the same plan is a no-op; interrupted apply (raise after N steps) resumes to the same end state; free-space check refuses with the shortfall (FR-003, FR-018, FR-019, SC-004)
- [x] T022 [US2] Implement those guards in `applier.py` and wire `apply`/`restore` in `__main__.py`, writing `.repair/results-<id>.json` on completion, until T021 passes

**Checkpoint**: the ~1,770 duplicates can be removed safely and reversibly.

## Phase 5: User Story 3 — Tidy Artist / Album / Track structure and names (P2)

**Goal**: tracks end at `Artist/<Album|Singles>/NN - Title.mp3` with cleaned titles and merged artist folders; colon folder folded.
**Independent test**: fixture `50 Cent/Old TikTok…/50 Cent - In Da Club (Official Music Video).mp3` → `50 Cent/Singles/In Da Club.mp3`; the colon folder is merged into its clean twin.

- [x] T023 [P] [US3] Write failing planner tests: a settled file whose lookup status is `not_looked_up`/absent is re-planned in place (never as a playlist source) while other settled files are not; an album folder `Coldplay/Coldplay - Parachutes/` (all tracks by that artist) keeps its tracks as `Coldplay/Parachutes/Title.mp3` on no match, is also a playlist, and a confident match to another album wins; no-match tracks keep the (cleaned) uploader folder and go to `Singles/<Title>.mp3` (no NN); confident matches go to `<LeadArtist>/<Album>/NN - Title.mp3` (`Chief Keef` example moves out of `50 Cent/`); `X` and `X, Y, Z` are NOT merged on similarity; two tracks resolving to one path → `conflict` action and neither moves; `Coldplay: Everyday Life` → `merge_dir` into `Coldplay - Everyday Life`; original title kept in `SYNODL_ORIGINAL_TITLE`; no produced name contains a mount-hostile character — FR-008, FR-009, FR-010, SC-005
- [x] T023a [P] [US3] Write failing planner+applier tests for FR-016: `album.nfo` in a playlist folder that ends up with no audio becomes an `orphan_nfo` action (moved to `.trash/<id>/`); `artist.nfo`, `folder.jpg`, `logo.png`, `backdrop.jpg` stay in their artist folder; when two artist folders merge, the surviving folder's `artist.nfo` wins and the other is orphaned; NO `.nfo` whose audio still exists is ever touched; `.ytdlp-archive.txt` is byte-identical after a full apply (SC-008)
- [x] T024 [US3] Extend `planner.py` with `move`, `merge_dir`, `conflict`, `orphan_nfo` actions and `retag` (title, artist, albumartist, featured, `SYNODL_ORIGINAL_TITLE`, `SYNODL_REPAIR`) until T023/T023a pass
- [x] T025 [US3] Write failing `test_applier.py::TestMoveAndRetag`: move + retag write the temp file, fsync, rename (never overwrite), mtime kept, tags read back correctly with mutagen, previous tag values are journaled before overwrite, `SYNODL_REPAIR` set so a second `plan` yields zero actions (SC-004), empty source folders left in place then removed only if empty and ours
- [x] T026 [US3] Implement `move`, `retag`, `merge_dir`, `orphan_nfo` in `applier.py` until T025 passes
- [x] T027 [US3] Add a whole-library idempotence test to `test_cli.py`: `plan → apply → plan` on the fixture yields an empty action list; and the SC-001/SC-002 invariants hold after apply: every video id present before is present after or under `.trash/<id>/`, no id appears on two audio files, every `Playlists/*.m3u8` entry resolves to a real file (C4)

## Phase 6: User Story 4 — Real metadata and cover art, only where trustworthy (P2)

**Goal**: MusicBrainz + Cover Art Archive, then iTunes, then Deezer; strict confidence; cached; failures reported as `not_looked_up`.
**Independent test**: fake sources; a confident match is tagged with album/track/year/ids + cover; a near-miss stays `Singles`.

- [x] T028 [P] [US4] Write failing tests `test_sources.py` (clients) against a local fake server serving canned MusicBrainz / CAA / iTunes / Deezer JSON captured in research R4: MB search filtered by length then release browse then track number; CAA 404 = no art and follows the two `archive.org` hops; iTunes artwork url upsized; Deezer plain query + `/track/<id>`; order MB→iTunes→Deezer, first confident wins; source 5xx/timeout → `not_looked_up`, not `no_match` (FR-011, FR-013)
- [x] T028a [P] [US4] Add an opt-in live smoke test `scripts/music_repair/test_live_sources.py` (runs only with `MUSIC_REPAIR_LIVE=1`, never in CI): one known song per source parsed by the real clients, so an upstream API change is noticed by the operator before a two-hour run (U2)
- [x] T029 [P] [US4] Write failing cache tests in `test_sources.py`: `.repair/cache.json` keyed by video id, written atomically and every 25 lookups so a lost pod resumes, hit avoids network, `not_looked_up` never cached, corrupt cache file is ignored not fatal
- [x] T030a [US4] Match-rate measurement (read-only, before anyone starts a long run): with the real clients and a scratch cache, look up ~50 random tracks from `/Volumes/music`; report matched / no-match / album-known counts to the user in the spec's verification notes. Do NOT loosen the ±3 s rule — report the number
- [x] T030 [US4] Implement the source clients + `Lookup.resolve(track) -> Match|None|NotLookedUp` and the cache in `scripts/music_repair/sources.py` until T028/T029 pass
- [x] T031 [US4] Extend `planner.py` to take Match results (album, track_no, year, ids, cover) into `retag`/`cover` actions and record `source` and the comparison in each action's reason (FR-012); tests in `test_planner.py` incl. the compilation-only case (album `Singles`, ids + year still written) and `not_looked_up` summary
- [x] T032 [US4] Write failing `test_applier.py::TestCover`: cover downloaded through the guarded fetcher, embedded as APIC front cover, `folder.jpg` written beside the album, image type sniffed by magic bytes not extension, oversize/non-image response rejected
- [x] T033 [US4] Implement `cover` in `applier.py` until T032 passes

## Phase 7: User Story 5 — Leftovers repaired (P3)

**Goal**: `.webm` → mp3 or reported; `.bin` given its type or set aside.
**Independent test**: a `.webm` with audio converts to a tagged mp3; one without is reported and left.

- [x] T034 [P] [US5] Write failing tests: planner emits `convert` for `.webm` with an audio stream and `skip`(reason) for one without (ffprobe stubbed), `rename_bin` for image magic bytes else `trash`; applier `convert` calls ffmpeg with `libmp3lame`, then trashes the original; converted file gets identity/naming like any track — FR-017
- [x] T035 [US5] Implement `convert` and `rename_bin` in `planner.py` and `applier.py` until T034 passes

## Phase 8: Delivery and cross-cutting

- [x] T035a [P] Add the security review test `scripts/music_repair/test_cli.py::TestOutbound`: over a full fixture run with a recording transport, every request host is allowlisted and HTTPS, and the request text contains only artist/title (SC-009)
- [x] T036 [P] Write `deploy/k8s/music-repair-job.yaml`: Job template (image and UID/GID substituted from `synodl-config`, `restartPolicy: Never`, `backoffLimit: 0`, `activeDeadlineSeconds: 43200`, `automountServiceAccountToken: false` (FR-014: no cluster API access), `runAsUser/Group`, resource limits, `ttlSecondsAfterFinished`, ONE volume = `synodl-music` PVC at `/library`, ConfigMap code volume at `/opt/music_repair`, `PYTHONPATH=/opt`)
- [x] T037 Write `scripts/music-repair.sh` (`plan | apply <id> | status | restore <id>`, builds the ConfigMap from `scripts/music_repair/*.py` and `names_cases.json`, excluding tests, waits, streams logs, exit codes per contract, refuses `apply` with no id) and `scripts/music_repair/test_cli.py::TestShell` asserting argument handling and the rendered manifest (`kubectl … --dry-run=client -o yaml` optional, skipped when kubectl absent)
- [x] T038 [P] Add the python test step to `.github/workflows/ci.yml`, running in the pinned worker image with `MUSIC_REPAIR_REQUIRE_DEPS=1` (and its path filter in `.github/path-filters.yml` so tool-only changes run it), and keep it OUT of the heavy jobs
- [x] T039 [P] Docs: `docs/MUSIC-LIBRARY-REPAIR.md` (the runbook from `quickstart.md`, snapshot-first warning, restore, expectations about how many tracks stay in `Singles` per research R5; that dot-folders `.trash`/`.repair` are normally ignored by Jellyfin/Plex and how to exclude `Playlists/` from the music library if it shows up as an artist (D2)); one-paragraph pointer in `CLAUDE.md` and a line in `deploy/k8s/README.md`
- [x] T040 Security/behaviour review against the spec's Credential-Safety Impact: grep that no log call can print a URL query, tag description or env; test asserting log output of a full fixture run contains none of the fixture descriptions or a `purl`
- [x] T041 Real-library verification (read-only; scope guard: NO Jobs in the home cluster and nothing written to the live share — hand the operator the `plan`/`apply` commands): run `python3 -m music_repair plan --library /Volumes/music --repair-dir <scratch> --no-lookup`, compare totals with the survey (5,766 mp3, ~1,770 duplicates, 1 colon folder, 2 webm, 2 bin), spot-check 50 Cent, Coldplay, `$uicideboy$`, Coffee Jazz Melody; record numbers in the spec
- [x] T042 Gate: python unittest, `cd server && go build ./... && go vet ./... && go test ./...`, `npm run build`, `npm run test:unit:coverage` (nothing else should change; prove it)
- [x] T043 Commit in logical chunks (Conventional Commits, non-user-facing types), push `feat/1052-repair-music-library`, set spec `**Status**: in-review` and `make roadmap`; do NOT open a PR unless asked

## Dependencies

- Phase 1 → Phase 2 → everything. T003→T004, T005→T006, T007→T008, T009→T010.
- US1 (T011–T014) needs Phase 2. US2 needs US1's planner skeleton. US3 needs US2's planner + applier core. US4 needs US3's `retag`. US5 needs the applier core (T020) and is otherwise independent of US3/US4.
- T036–T039 need the CLI (T013, T022). T041 needs US1–US3 minimum.

## Parallel opportunities

- Phase 2: T003, T005, T007, T009 (four test files) in parallel; then T004, T006, T008, T010.
- US2: T015 ∥ T016. US4: T028 ∥ T029. Phase 8: T036 ∥ T038 ∥ T039.

## Implementation strategy

MVP = Phases 1–3 (safe audit). Then US2 (biggest win, most risk, so it gets the safety tests first),
US3, US4, US5. The operator only runs `apply` after reading a plan, so each story is shippable as it lands.

# Implementation Plan: Repair the music library

**Branch**: `feat/1052-repair-music-library` | **Date**: 2026-09-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/1052-repair-music-library/spec.md`

## Summary

An **operator tool**, not a server feature: a Python package `scripts/music_repair/`
(stdlib + `mutagen` + `ffprobe`/`ffmpeg`, all already in the pinned worker image)
that plans and applies the repair, delivered to the cluster as a short-lived
Kubernetes Job that mounts exactly the one music library claim. Tests run inside the pinned worker image, so the production runtime is what is tested. It follows the
precedent of `scripts/library_tidy.py`: a pure, unit-tested planner; dry-run as the
default; a reviewable plan; nothing deleted; replayable.

Two commands, both wrapping `kubectl`:

```
scripts/music-repair.sh plan            # Job #1 → .repair/plan-<id>.{json,md}   (changes nothing)
scripts/music-repair.sh apply <id>      # Job #2 → carries out exactly that plan
```

No Go code, no `/v1` endpoint, no UI, no new server permission (FR-001, FR-014).

## Technical Context

**Language/Version**: Python 3.14 (what the pinned worker image ships); stdlib-first.

**Primary Dependencies**: `mutagen` 1.48 (ID3 read/write, cover embed), `ffprobe`/`ffmpeg` with
`libmp3lame` (duration, `.webm` → mp3). All present in `jauderho/yt-dlp:2026.08.19`
(verified by running the image). **No new dependency in Go or npm.**

**Storage**: Files only, on the ONE library volume: `.repair/` (plans, results, lookup
cache), `Playlists/`, `.trash/`. No database, no second datastore.

**Testing**: `python3 -m unittest discover -s scripts/music_repair -p 'test_*.py'`. Pure planner
tables + an `httptest`-style fake for the four sources (a local `http.server` fixture, HTTPS
guard bypassed only through an injected fetcher, never a flag). A fixture library
generator builds real tiny mp3s with `ffmpeg` so tag round-trips are tested, skipped
cleanly where ffmpeg/mutagen are absent. Wired into CI.

**Target Platform**: k3s Job (linux/amd64), library mounted RWX from NFS (`synodl-music` claim).

**Project Type**: operator CLI run as a one-shot Job.

**Performance Goals**: full pass over ~5,800 tracks: inventory < 10 min; lookups bounded by
MusicBrainz's 1 req/s (~2 h once, then cached); apply is renames within one volume (fast).

**Constraints**: dry run writes only under `.repair/`; free-space check before apply;
resumable + idempotent; hosts fixed to an allowlist; HTTPS only.

**Scale/Scope**: 5,766 tracks, 1,888 artists, 3,497 folders, ~3,100 distinct songs.

## Constitution Check

| Principle | Status |
|---|---|
| I Spec-driven | Spec 1052; full pipeline followed. |
| II TDD | Pure planner/names/identity/lookup modules get tests written first (tasks order Red→Green). No handler/session logic touched, so no Go/e2e change is required; the CLI wrapper gets a shell-level dry test. |
| III Custodial state & credentials (NON-NEGOTIABLE) | **No SynoDL state is touched.** No second datastore: the cache is a JSON file on the *media* volume holding derived public facts (video id → match), no secret, no user id — the same category as `person_photos`, and it lives with the operator's media, not in SQLite. No credentials involved: the four sources are keyless. See *Credential-Safety Impact* below. |
| IV Offline-first client | N/A (no client change). |
| V Quality gates | `go build/vet/test` and `npm run build` unaffected but still run; new python gate added to CI. Commit type is `chore(tools)`/`docs`, not user-facing, so it never reaches "What's new". |
| VI Ionic-first UI | N/A. |
| VII Traceable delivery | Issues per task group, `Closes #N` in the PR. |
| Domain: ephemeral workers only | The Job starts, does one unit of work, exits; holds no SynoDL state; created by the OPERATOR with kubectl, so the server's RBAC is **unchanged** (no widening of the namespaced Role). |
| Domain: media volumes are worker-only | The Job (a worker-class pod) mounts the library; the server container still does not. |
| Domain: worker images pinned | Reuses the pinned `YTDL_IMAGE` tag from `10-synodl.yaml`; no `:latest`. |

Gate result: **PASS**, no Complexity Tracking entries.

### Credential-Safety Impact

- **Stored / protected**: nothing is added to SynoDL's store. Files written to the library:
  plans (paths + tag values), results, a lookup cache of public metadata. None contain a
  secret.
- **Crosses to the NAS**: nothing goes to the DSM API. The Job writes to the NFS export
  through the library mount only.
- **Outbound**: only the four public sources, HTTPS, over a fixed allowlist (`FR-015`) with
  redirect hops re-checked. Requests carry a User-Agent naming SynoDL and a contact, and the
  query text (artist, title). They carry no credential, session id, user id or account data.
- **Logs / errors**: worker output records counts and relative library paths. It MUST NOT
  log full request URLs with query strings beyond host + path class, tag descriptions
  (video descriptions can contain personal links), or any environment variable.
- **Why safe**: least privilege is the operator's own kubectl RBAC; the Job's ServiceAccount
  has no API permissions (`automountServiceAccountToken: false`).

## Project Structure

### Documentation (this feature)

```text
specs/1052-repair-music-library/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli.md            # commands, flags, exit codes
│   └── plan-file.md      # plan.json schema
├── checklists/requirements.md
└── tasks.md              # /speckit-tasks
```

### Source Code

```text
scripts/
├── music-repair.sh                 # kubectl wrapper: plan | apply <id> | status | restore
└── music_repair/
    ├── __init__.py
    ├── __main__.py                 # CLI entry (argparse) — python3 -m music_repair
    ├── names.py                    # title cleaning, artist cleaning, sanitising (pure)
    ├── inventory.py                # walk library → Track records (mutagen/ffprobe)
    ├── identity.py                 # video id → Song; choose kept copy (pure)
    ├── sources.py                  # host allowlist, guarded fetch, 4 source clients, cache
    ├── matching.py                 # confidence rule (pure)
    ├── planner.py                  # Track+Match → Plan (pure)
    ├── playlists.py                # .m3u8 render/merge (pure)
    ├── applier.py                  # execute a Plan: moves, tags, covers, trash, convert
    ├── report.py                   # plan.md / results rendering (pure)
    ├── test_names.py test_identity.py test_matching.py test_sources.py
    ├── test_planner.py test_playlists.py test_applier.py test_cli.py
    └── fixtures.py                 # builds a tiny fake library for tests
deploy/k8s/
└── music-repair-job.yaml.tpl       # Job template: one PVC, pinned image, ConfigMap code
.github/workflows/ci.yml            # + python unittest step
```

**Structure Decision**: an operator tool beside `scripts/library_tidy.py`, delivered as a
Job. The Go server is untouched — the spec's "worker in the cluster, never in the server
process" is met by the Job, and "no endpoint, no UI" (clarified) by not touching the API.

## Phases (build order)

1. **Pure core** (tests first): names, identity, matching, playlists, planner over fixtures.
2. **Inventory + sources**: mutagen/ffprobe reading; guarded fetch + allowlist + fakes for
   MusicBrainz / Cover Art Archive / iTunes / Deezer; cache.
3. **Applier**: trash-first moves, retag + cover embed, `.webm` convert, `.bin` fix,
   `Playlists/` write, free-space gate, resumable journal, `restore`.
4. **CLI + Job**: `__main__`, `music-repair.sh`, `music-repair-job.yaml.tpl`, docs, CI step.
5. **Real-library verification**: offline dry run on `/Volumes/music` (read-only, plan to a
   scratch dir), review counts against the survey; then hand the operator the two commands.

## Complexity Tracking

None.

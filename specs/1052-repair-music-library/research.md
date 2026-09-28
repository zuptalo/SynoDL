# Research: Repair the music library

Findings come from the live volume (`/Volumes/music`, read-only), from running the pinned
worker image, and from real calls to the four public sources on 2026-09-28. Nothing was
written to the library.

## R1 — Where does it run, and what does the image have?

**Decision**: a one-shot Kubernetes Job in the pinned worker image
(`jauderho/yt-dlp:2026.08.19`), mounting only the `synodl-music` claim; started by the
operator with `kubectl`, code delivered as a ConfigMap.

**Verified in the image**: Python 3.14.7, `mutagen` 1.48.1, `ffprobe`/`ffmpeg` 8.1.2 with
`libmp3lame`. No new image, no new dependency. The image runs as uid 0 by default, so the
Job pins `runAsUser/runAsGroup` to `YTDL_UID/YTDL_GID` (1000) exactly as workers do, so
files keep the ownership the media server expects.

**Why not the Go server / a `/v1` endpoint**: clarified out of scope (operator-run only),
and it would grant a request-serving process write access to the media volume the
constitution reserves for workers.

**Alternatives**: extend `internal/ytdl` (rejected — that is the download path; this is a
one-off library operation); run from the laptop over SMB (rejected — the mount cannot list
the colon folder, is slow, and cannot rename atomically over NFS semantics).

## R2 — Song identity

**Decision**: the 11-character YouTube id from the `purl` tag (`comment` as fallback),
parsed from `watch?v=<id>`. Verified on real files: `purl` and `comment` both hold the
watch URL. Files with neither are kept and reported (FR-004).

**Evidence that hashes cannot work**: the same song in two playlist folders differs in
size (7,019,644 vs 7,019,710 bytes) because the embedded `album` tag differs.

**Kept-copy rule** (clarified): largest size → earliest mtime → path.

## R3 — Reading tags fast on NFS

**Decision**: read ID3 with `mutagen` directly (`ID3(path)`, no audio decode); duration from
`mutagen.mp3.MP3(path).info.length`. `ffprobe` is used only for `.webm`. One pass, no
per-file subprocess for the 5,766 mp3s. The mp3s have an ID3v2 header with `TIT2, TPE1/TPE2,
TALB, TDRC, COMM, TXXX:purl`.

## R4 — The four sources (verified responses)

| Source | Query | Gives | Notes |
|---|---|---|---|
| MusicBrainz | `GET /ws/2/recording?query=artist:"A" AND recording:"T"&limit=25&fmt=json` | recording id, `length` (ms), `artist-credit[]`, a *few* releases | One hit **per recording version**. For "Yellow" the score-100 hits are live/compilation versions. So filter hits by exact artist+title and `length` within ±3 s FIRST. |
| MusicBrainz | `GET /ws/2/release?recording=<id>&inc=release-groups&status=official&limit=100` | releases with `release-group.primary-type` / `secondary-types`, `date` | Choose the album (see R5). |
| MusicBrainz | `GET /ws/2/release/<id>?inc=recordings&fmt=json` | `media[].tracks[].position/number` | Track number for the chosen release. |
| Cover Art Archive | `GET /release/<mbid>/front-500` | jpeg | 307 → `archive.org/download/...` → 302 → `dnNNNNNN.ca.archive.org/...`. **Two redirect hops through `archive.org`**: the allowlist MUST cover it, and each hop is re-checked. 404 = no art. |
| iTunes Search | `GET https://itunes.apple.com/search?term=<artist title>&entity=song&limit=10` | `trackName, artistName, collectionName, trackNumber, releaseDate, trackTimeMillis, artworkUrl100` | Swap `100x100bb` → `600x600bb` for art (host `*.mzstatic.com`). No key. |
| Deezer | `GET https://api.deezer.com/search?q=<artist title>` | `title, artist.name, duration (s), album.title, album.cover_xl` | The `artist:"x" track:"y"` advanced syntax returned no rows; use the plain query and verify strictly ourselves. Track number needs a second call `/track/<id>` (`track_position`, `release_date`). |

MusicBrainz requires a descriptive `User-Agent` and 1 request/second; the client
enforces both. iTunes and Deezer are used at low volume and only when MusicBrainz is not
confident, and everything is cached.

## R5 — Choosing the album for a confident recording

Probe: the 50 Cent "In Da Club" video (223 s) matches an MB recording whose only release
is a compilation ("Bravo Hits Party: 2000ER"). The album version is 193 s, so it is (correctly)
not confident against this video.

**Decision**: among the confident recording's official releases, take the release whose
release-group has `primary-type = Album` (or `EP`, `Single`) with **no secondary types**
(no Compilation, Live, Soundtrack, Remix, DJ-mix, Mixtape/Street…), earliest `date`, ties by
id — and **Album beats EP beats Single** (a song's own single predates its album, and a library
shelves by album). If none qualifies, the album is unknown: the song is still confident (ids and year are
kept) but is filed under `Singles`. **A compilation is never used as the album.**

**Expected effect (be honest with the operator)**: YouTube videos are often longer than the
album cut (intros, skits), so the ±3 s rule leaves many popular songs in `Singles` with a
clean title only. That is the spec's "never a guess" trade-off. The plan report shows the
count of "no confident match" and the runner-up's length so the operator can judge whether
to loosen the tolerance in a follow-up.

## R6 — Title and artist cleaning

Real inputs: `50 Cent - In Da Club (Official Music Video)`; `Chief Keef Feat 50 Cent & Wiz Khalifa - ＂Hate Bein' Sober＂`
(fullwidth quotes from yt-dlp's filename sanitising); `Double You - Please Don't Go [Official Video]`;
`SNAP! - The Power (Official 4K Music Video)`.

**Decision** (pure, table-tested):
1. NFC-normalise; map yt-dlp's fullwidth lookalikes (`＂ ｜ ： ⧸ ？`) back to `" | : / ?`
   for *comparison*, never for filenames.
2. Split `Artist - Title` on the first ` - ` when the left side plausibly is the artist
   (equals the tag artist, or contains it, or the tag is missing).
3. Remove trailing/embedded bracket groups whose words are ONLY from a noise vocabulary
   (official, music video, video, lyric(s), audio, visualizer, hd, hq, 4k, 1080p, remastered
   in HD, explicit, clean, dirty version…). `(Live)`, `(Remix)`, `(Acoustic)`,
   `(feat. …)`-less versions are **kept** — they are different recordings.
4. `feat./ft./featuring/feat` groups are lifted into `featured[]`, not left in the title.
5. The original title stays in the tag `SYNODL_ORIGINAL_TITLE`, satisfying "found back at
   its source" (spec 0012 FR-008) alongside `purl`.

## R7 — Sanitising names for the mount

The colon folder is unreadable over the mount. The repair reuses the recipe's rule
(`server/internal/ytdl/sanitize.go`: separators, control chars, dot-only tokens, 120-char
bound), extended for Windows-reserved characters `: * ? " < > |` → their fullwidth
lookalikes (the same substitution yt-dlp itself makes), trailing dots/spaces trimmed. A
Python port with a shared fixture table (`names_cases.json`) keeps the two implementations from
disagreeing; a Go test reading the same table is added in spec 1053 (the recipe change), which
owns changing `sanitize.go`.

## R8 — Settled files and idempotency

Every file the repair writes gets `TXXX:SYNODL_REPAIR=<planid>`. A **settled** file is not a
playlist member source (its old folder is gone) and is not re-planned — EXCEPT when its cached
lookup status is `not_looked_up` or it has no cache entry (a source was down during the first
run, FR-013): those re-enter lookup and are retagged/re-filed in place, still never counted as
playlist members.
Playlist files are **merged** into any existing `Playlists/<title>.m3u8`, so a later run over
new downloads only adds. Lookups are cached by video id in `.repair/cache.json`, so a second
run needs no network and reaches the same plan; a plan with zero actions is the
idempotence check (SC-004).

## R9 — Apply safety

- Free space (FR-019): sum of new bytes needed (conversions, cover files, m3u8) + 5% margin
  vs `shutil.disk_usage`; refuse with the shortfall.
- Ordering: write new/retagged file to a temp name in the destination dir, `fsync`, rename
  over nothing (never overwrite: target exists → conflict, both kept). Sources go to
  `.trash/<planid>/<original relpath>` by `rename` (same volume, atomic, restorable).
- Journal: `.repair/journal-<planid>.jsonl`, one line per completed step, so an
  interrupted apply resumes and `restore <planid>` can invert moves.
- Staleness (FR-003): every action carries its source's plan-time `(size, mtime_ns)`. The
  fingerprint is checked ONCE per source file, at the FIRST action that touches it; later
  actions on the same file (retag → move → cover) follow the file through the journal and do not
  re-check, because retagging legitimately changes size. Test: one file retagged and moved in
  one apply is not reported stale.
- Album folders (clarified): a folder named `<Artist> - <X>` whose tracks are all by that artist
  is an album; unmatched tracks keep it as `Artist/<X>/Title.mp3`, and it is still also a
  playlist. Detected in `names.is_album_folder(folder, artist)`.
- `.ytdlp-archive.txt`, `*.nfo` (except orphans), and `.repair/` itself are never touched by
  the moves.

## R10 — Delivery to the cluster

`scripts/music-repair.sh` renders `deploy/k8s/music-repair-job.yaml` (image and UID/GID read
from the live `synodl-config` ConfigMap so they cannot drift from the download workers),
creates the code ConfigMap from `scripts/music_repair/*.py` (excluding tests), waits, and
streams logs. The Job: `restartPolicy: Never`, `backoffLimit: 0`, `activeDeadlineSeconds: 43200` (the first lookup pass is
~2–3 h at 1 req/s and must not be killed at a default), `automountServiceAccountToken: false`,
`ttlSecondsAfterFinished: 86400`, resource limits,
one volume (`synodl-music` PVC) at `/library`.

## R11 — Surviving a lost pod

The cache is written atomically (temp + rename) and incrementally — every 25 lookups — so a
replaced pod resumes from where it was. The ConfigMap carries `names_cases.json` with the `.py`
files (flat keys). Tests run in the pinned image so mutagen/ffmpeg are always present:
`docker run --rm -v "$PWD/scripts:/s" -w /s --entrypoint python3 jauderho/yt-dlp:2026.08.19 -m unittest discover -s music_repair -p 'test_*.py'`, with `MUSIC_REPAIR_REQUIRE_DEPS=1` turning a missing
dependency into a failure instead of a skip.

## Open items carried to tasks (none block)

- Tolerance is fixed at ±3 s per clarification; a `--tolerance` flag is intentionally NOT
  added in this spec.
- The `Playlists/` folder name may collide with an artist called "Playlists": the planner
  reports it and files that artist as `Playlists (artist)`.

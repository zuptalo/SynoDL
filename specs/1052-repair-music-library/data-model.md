# Data Model: Repair the music library

Nothing here is SynoDL state. Every structure is a file on the media volume, under
`.repair/` unless noted. None holds a secret or a user id.

## Track (inventory record, in memory + `inventory` in the plan)

| Field | Meaning |
|---|---|
| `relpath` | path relative to the library root, NFC-normalised for comparison only |
| `video_id` | 11-char id from `purl`/`comment`, or `null` |
| `size`, `mtime_ns` | staleness guard (FR-003) and kept-copy rule |
| `duration_s` | from the MP3 header |
| `tag_title`, `tag_artist`, `tag_album` | as found |
| `folder_artist`, `folder_playlist` | the two path components, as found |
| `settled` | true if `TXXX:SYNODL_REPAIR` is present; a settled track whose lookup status is `not_looked_up`/absent is re-planned in place |
| `lyrics` | sidecar `.lrc` relpath or `null` |

## Song (identity)

Grouped by `video_id`. One `kept` Track (largest size → earliest mtime → path) and `copies[]`
(trashed). `playlists[]` = the set of `folder_playlist` titles of all *unsettled* copies,
title-normalised (case, whitespace) per FR-006. A Track without a `video_id` is its own
Song with `identity: "none"`, kept, and listed under `unidentified`.

## Match

`source` (`musicbrainz|itunes|deezer`), `artist`, `featured[]`, `title`, `album|null`,
`track_no|null`, `year|null`, `mbid_recording|null`, `mbid_release|null`, `cover_url|null`,
`length_s`, plus the comparison that made it confident (`artist_eq`, `title_eq`,
`delta_s`). A Song has a Match only if confident (exact artist and title after
normalisation AND `|delta_s| <= 3`). Status per Song: `matched | no_match | not_looked_up`
(FR-013: a failing source yields `not_looked_up`, retried on the next run).

## Action (one plan step)

`{id, kind, reason, src, dst, size, mtime_ns, ...}` where `kind` is one of:

| kind | Effect |
|---|---|
| `move` | rename to `dst` (Artist/Album/NN - Title.mp3) |
| `retag` | write ID3 (title, artist, albumartist, album, track, year, ids, `SYNODL_*`) |
| `cover` | embed cover + write `folder.jpg` beside the album |
| `trash` | move a duplicate / orphan to `.trash/<planid>/<relpath>` |
| `convert` | `.webm` → mp3 via ffmpeg (then trash original) |
| `rename_bin` | `logo.bin` → real extension, or trash if not an image |
| `sidecar` | a `.lrc` moves to sit beside its (moved) audio; carries `audio` = the audio's final path |
| `orphan_nfo` | a media-server `.nfo` whose audio no longer exists → `.trash` (FR-016); never one whose audio remains |
| `merge_dir` | fold `Coldplay: Everyday Life` into `Coldplay - Everyday Life` |
| `playlist` | create/merge `Playlists/<title>.m3u8` |
| `conflict` | two things want one path: neither moved, reported |
| — | *Not actions:* a file left alone is listed in the plan's `skipped` array (unidentified, unreadable, symlink, no audio stream) with its reason. Files already correct produce nothing, which is what makes a second run empty (SC-004). |

## Plan (`plan-<id>.json`)

`{version, id, created, library, library_fingerprint, totals, actions[], skipped[], unidentified[]}`
See `contracts/plan-file.md`. The human copy `plan-<id>.md` is generated from it; apply
reads only the JSON.

## Journal (`journal-<id>.jsonl`)

One line per completed action: `{action_id, status: done|skipped|failed, at, note, previous_tags}` — `previous_tags` holds the value of every tag the step overwrote, so `restore` can revert tags as well as moves (FR-007).
Resume = skip action ids already `done`. `restore` replays `move`/`trash` in reverse.

## Lookup cache (`cache.json`)

`{ "<video_id>": {status, match|null, looked_up_at, sources_tried[]}, ... }` and
`{ "cover:<mbid|url-hash>": {saved_as} }`. Public facts only. `not_looked_up` entries are
never cached, so the next run retries them.

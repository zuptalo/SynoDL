# Repairing the music library

An **operator tool** (spec 1052) for a music library filled by SynoDL's YouTube
downloads. It is not part of the server and adds no endpoint or screen: you run it
with `kubectl`, it runs once as a short-lived Job that mounts only the music
library, and it exits.

## What is wrong with the old layout, and what it does about it

The download recipe filed every track as `Artist / <YouTube playlist title> / <video title>.mp3`.
That stores the same song once per playlist it appeared in, uses playlist titles as
albums, and names artist folders after uploaders (`10ccVEVO`). The repair:

| It does | How |
|---|---|
| One copy of each song | A song **is** its YouTube video id, read from the file's own tags (`purl`). File hashes cannot be used — copies differ, because the embedded album tag differs. The largest copy is kept (then earliest, then path). A file with no id is kept and reported, never merged on a guess. |
| Keeps playlists | Every folder title becomes one `Playlists/<title>.m3u8`, library-wide (the same title under fifty artists is **one** playlist), ordered by artist then title — the original order is not recoverable. Paths are relative, so it still resolves if the library is mounted elsewhere. |
| Two videos of one song | "Easy On Me (Official Video)" and "(Official Lyric Video)" are two video ids, so both are kept. The one already there (else the first by video id) keeps `Easy On Me.mp3`; the other becomes `Easy On Me [<video id>].mp3`. The title *tag* stays clean. Merging them would be a guess, so the repair does not. |
| `Artist / Album-or-Singles / NN - Title.mp3` | Titles lose "(Official Music Video)" noise; `(Live)`, `(Remix)` and `(Acoustic)` stay, because they are different recordings. |
| Real metadata | MusicBrainz (+ Cover Art Archive), then iTunes Search, then Deezer — first **confident** answer wins. Album, track number, year, MusicBrainz ids, cover. |
| Fixes leftovers | `Coldplay: Everyday Life` (a name the mount cannot list) folds into its twin; a `.webm` that carries audio becomes an mp3; a `.bin` gets its real image type or is set aside. |

**"Confident" is strict on purpose:** artist and title equal after normalising case
and punctuation, *and* the recording's length within 3 seconds of the file's. A
wrong album is worse than none.

> **Expect many songs to stay in `Singles`.** A YouTube video is often longer than
> the album cut (an intro, a skit), and the length rule is what tells "In Da Club"
> the album track from "In Da Club" the video. Where no source is confident the
> track keeps its uploader folder — cleaned of `VEVO`/`Official`/`- Topic` — and
> goes to `Singles`, except that an existing real album folder
> (`Coldplay/Coldplay - Parachutes/`) keeps its tracks as `Coldplay/Parachutes/`.
> The plan reports how many songs are unmatched and by how much their nearest
> candidate missed, so you can judge before applying anything.

## Before you start

1. **Snapshot the music share on the NAS** (Snapshot Replication). `.trash` protects against a
   *wrong plan*, not against a lost volume.
2. The Job runs the **pinned worker image** (`YTDL_IMAGE`) as `YTDL_UID:YTDL_GID` against the
   `YTDL_MUSIC_CLAIM` volume, all read from the live `synodl-config`, so it cannot drift from the
   downloads. Nothing to configure.

## Running it

```sh
scripts/music-repair.sh plan                       # ~2–3 h the first time (MusicBrainz allows 1 request/second)
#   prints  plan id 20260928T201500Z-a1b2c3
```

`plan` changes **nothing** in the library except its own folder `.repair/`. Read
`.repair/plan-<id>.md` on the share: totals, every conflict, every file left alone and
why, the largest duplicates, the no-match list, and 20 samples of each kind of change.

```sh
scripts/music-repair.sh apply 20260928T201500Z-a1b2c3   # carries out exactly that plan
scripts/music-repair.sh status                          # the last Job's log
scripts/music-repair.sh restore 20260928T201500Z-a1b2c3 # undo it
```

Then let the media server rescan.

If a `plan` or `apply` Job is stopped (a deadline, a node drain), run the same command again: `plan` resumes from its
saved lookups and `apply <id>` continues from its journal. A file is only ever the complete old version or the
complete new one — tags are rewritten on a copy and swapped in atomically.

### What makes it safe

- **Nothing is deleted.** Removed files go to `.trash/<plan id>/`, keeping their path.
  Nothing empties `.trash` — that is yours to do once you are happy.
- **Nothing is overwritten.** A destination that exists fails that step; the source stays.
- **Plans are inputs, not authority.** Every path must be library-relative and cross no symbolic
  link, or the whole plan is refused before the first step.
- **Stale steps are skipped.** A file that changed since the plan was made (a download landed)
  is skipped and reported, never forced.
- **Resumable and idempotent.** Every step is journaled (`.repair/journal-<id>.jsonl`), including
  the previous value of every tag it rewrote. Interrupted → run `apply <id>` again. Run twice →
  the second finds nothing. Songs whose lookup could not finish (a source was down) are retried
  by the next `plan`, in place.
- **`restore <id>`** puts trashed files back and the previous tags and cover art back. An entry
  whose original location is now occupied is skipped and reported, never overwritten.
- **One run at a time.** A lock left by a crashed run is *reported* with who and when; remove
  `.repair/lock` yourself once you are sure that run is gone.
- **Free space** is checked before apply, with the shortfall stated.
- `.ytdlp-archive.txt` is never touched, so nothing already downloaded is fetched again.
  Media-server `.nfo` files are left alone unless their tracks are gone.

### What it sends over the network

Only HTTPS to `musicbrainz.org`, `coverartarchive.org` (+ the `archive.org` hosts it redirects
to), `itunes.apple.com` (+ `mzstatic.com`), and `api.deezer.com` (+ `dzcdn.net`). A request
carries the artist and title being looked up and a `User-Agent` naming SynoDL — nothing else:
no credential, no session, no account. Every redirect hop is re-checked; a look-alike host
(`evilarchive.org`) is refused. Logs contain counts and library-relative paths only — never
tag contents (video descriptions can hold personal links), URLs or environment variables.

### Media servers

`.trash/` and `.repair/` are dot-folders, which Jellyfin and Plex are expected to ignore. `Playlists/` holds
`.m3u8` files. **Not verified against your server:** whether it imports them as playlists automatically, needs
an import step, or lists the folder as an artist. Check after the first `apply` (and exclude `Playlists/` from the
music library if it appears as an artist). The songs themselves are unaffected either way.

## From Settings (admins)

Administrators can run the same repair without `kubectl`: **Settings → Music library
repair**.

1. **Check library** starts a dry run (changes nothing) and shows progress, then counts:
   duplicates, moves, retags, playlists, conflicts, matched songs, songs staying in
   Singles, space needed versus free, and what is left alone and why (with examples).
2. **Apply** is offered only for a finished check less than 24 hours old, behind a
   confirmation you must tick that says what will happen and that a snapshot of the
   share is advisable. Interrupted or partly failed runs can be continued.
3. **Undo** reverses the latest apply (files return from `.trash` and old paths).
4. **History** lists who started what, when, and the outcome.

Only one repair runs at a time, including one started with `kubectl`. The worker runs
the server's own version of the tool (copied from its image), the server never mounts
the library, and it learns about a run only from bounded, parsed progress/result lines
the worker prints. Nothing else about the operator workflow changes; the scripts keep
working.

## Local, read-only check

```sh
python3 -m music_repair plan --library /Volumes/music --repair-dir /tmp/repair --no-lookup
```

(`--repair-dir` outside the library means nothing is written to it.) Tests run inside the pinned
image, exactly as CI does: `scripts/music-repair-test.sh`.

## Later

Changing what NEW downloads write, so they stop recreating the layout, is a separate spec
(1053). Until then, run `plan` again after a batch of downloads: it only touches what is new.

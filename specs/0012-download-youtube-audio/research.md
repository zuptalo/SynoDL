# Research: Save YouTube music and music videos to the library

**Spec**: [spec.md](./spec.md) | **Date**: 2026-09-06

Every question below was settled by running the real thing against real content
before the plan was written. What follows records the decision, why, and what was
rejected — so the implementation does not re-litigate it.

## 1. Kubernetes access from the server

- **Decision**: a small stdlib-only client in `server/internal/k8s`. In-cluster
  config comes from `KUBERNETES_SERVICE_HOST`/`KUBERNETES_SERVICE_PORT` plus the
  projected ServiceAccount files (`token`, `ca.crt`, `namespace`). Three
  operations only: create a Job, list Jobs by label selector, delete a Job. JSON
  over `net/http` with the CA in a `tls.Config`.
- **Rationale**: matches the repo's stdlib-first style and its rule that every Go
  module is a spec-level decision. It is testable against `httptest` exactly as
  `internal/syno` is tested against a fake DSM.
- **Alternatives rejected**: `client-go` (disproportionate dependency tree for
  three calls, plus a version-skew policy tied to cluster releases); shelling out
  to `kubectl` (a binary dependency and a shell surface, both worse).

## 2. Video quality: progressive vs. merged — MEASURED

- **Decision**: request the best video and best audio separately and mux them
  losslessly: `-f "bv*[vcodec^=avc1]+ba[acodec^=mp4a]/bv*+ba/b"` with
  `--merge-output-format mp4`.
- **Evidence**: for a representative track, the ONLY format carrying both picture
  and sound was `18` (640×360). Everything from 480p to 1080p is video-only. And
  format `18` is not reliably offered: across four identical requests it was
  present twice and absent twice, so a progressive-only selector fails at random
  with "Requested format is not available".
- **Merging is not a conversion**: probing the merged output gave
  `h264 High 1920×1080` + `aac LC 44.1 kHz stereo`, i.e. the two source streams
  copied verbatim into one container. No re-encode, no quality loss, no CPU cost
  beyond I/O. This satisfies the user's "no extra conversions" intent while
  removing the 360p ceiling that a literal reading would have imposed.
- **Alternatives rejected**: progressive-only (360p ceiling AND intermittent);
  VP9/Opus at the same resolution (equally lossless, but H.264/AAC direct-plays
  on far more Plex clients).

## 3. Original-language captions

- **Decision**: `--sub-langs ".*-orig"` with `--write-auto-subs`.
- **Rationale**: the value is a regex, and it resolves the source's
  original-language auto-caption track with no per-item inspection — so no
  metadata pre-pass is needed before building the command.
- **Alternatives rejected**: a two-pass "inspect then download" flow (doubles the
  requests and adds a failure mode for a value the regex derives for free);
  hardcoding a language (wrong for anything not in that language).

## 4. Companion-file naming differs by media type — MEASURED

- **Decision**: audio gets `<base>.lrc`; video gets `<base>.<lang>.srt`. Both are
  produced by a single constant `--exec after_move` snippet that renames what the
  downloader wrote.
- **Rationale**: the downloader always names companion files
  `<base>.<lang>.<ext>` and offers no option to omit the language infix. Plex and
  Jellyfin match **lyrics** by exact basename, so the infix must go; they match
  **video subtitles** by a 2-letter language suffix, so `en-orig` must become
  `en`. Verified: with the rename, the files land as `…​.lrc` and `…​.en.srt`.
- **Alternatives rejected**: embedding lyrics in the audio file instead of a
  sidecar — sidecars are what both media servers read, and embedding would need a
  tag-writing step (ffmpeg cannot do it: `-metadata lyrics-eng=` writes a
  non-standard `TXXX` frame, not a real `USLT` one).

## 5. Shelving depends on tags, not folders — MEASURED

- **Decision**: pair the output template with two `--parse-metadata` flags that
  write album and album-artist from the *same* template.
- **Evidence**: with metadata embedding alone, the saved file carried title and
  artist but an EMPTY album and album-artist. Plex reads tags in preference to
  folder names, so every track would have been shelved under *[Unknown Album]*
  despite sitting in a correctly named folder. With the two flags, the tags come
  out populated and consistent with the path.
- **Trap to avoid, recorded**: do NOT add the playlist title to that fallback
  chain. On a channel download it resolves to the channel's tab name, which would
  tag every track with that as its album.

## 6. Channel scope is a URL choice, not a filter — MEASURED

- **Decision**: for a channel, target the channel's videos tab rather than the
  bare channel URL, and apply a minimum-duration filter as a secondary guard.
- **Evidence**: a bare channel URL expands to the Videos and Shorts tabs (20 and
  61 items for the channel measured) and never touches the Playlists tab.
  Targeting the videos tab excludes every Short *without enumerating them*, which
  is both correct and much cheaper than fetching metadata for 61 items to reject
  them. The duration filter then only has to catch stragglers, and it is
  evaluated from the listing itself with no extra requests.
- **Also measured**: the channel's playlists added nothing downloadable — the
  seven items they contained that the tabs did not were all private or otherwise
  unavailable. Playlist grouping is therefore not a gap worth closing here; a
  playlist submitted directly is still handled as its own scope.
- **Duration bound is one-sided**: short items are excluded, long ones never are —
  a 41-minute continuous mix on the measured channel is legitimate content.

## 7. Idempotent re-runs

- **Decision**: `--download-archive` kept inside the target library.
- **Evidence**: verified by running the same playlist twice — the second run
  reported every item as already recorded and downloaded nothing.
- **Consequence**: the archive lives per-library, so the same item can exist as
  both audio and video without either run suppressing the other.
- **Note**: items rejected by the duration filter are not written to the archive,
  so they are re-evaluated (not re-downloaded) on each run — which is the desired
  behaviour if a clip is ever replaced by a full-length upload.

## 8. Mock orchestrator for dev and e2e

- **Decision**: a new `server/cmd/synok8s` binary implementing just enough of the
  Jobs API surface the client uses, with `/__mock/*` control endpoints to drive a
  Job through started → succeeded/failed on command. It never downloads anything.
- **Rationale**: directly mirrors `cmd/synomock`, which already proves the
  pattern; keeps `make start` and e2e hermetic as the Domain Constraints require;
  and exercises the real wire format, label selector, and state mapping — the
  three places this feature is most likely to break.
- **Alternatives rejected**: faking only at the Go interface boundary (leaves the
  wire contract untested, which is the actual risk); testing against a real
  cluster (breaks dev parity and hermetic e2e).

## 9. File ownership

- **Decision**: run the worker with the library's uid/gid and set
  `XDG_CACHE_HOME=/tmp`.
- **Rationale**: verified that this produces files owned by the invoking user
  rather than root, so no permission repair is needed before the media server or
  the operator can manage them. The cache variable keeps the downloader from
  attempting to write a home directory that does not exist in the pod.

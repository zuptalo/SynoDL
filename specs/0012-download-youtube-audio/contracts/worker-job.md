# Contract: Worker Job

**Spec**: [spec.md](../spec.md) | **Date**: 2026-09-06

The exact Job SynoDL creates. Every flag here was verified end-to-end against
real content before this spec existed (see [research.md](../research.md)); this
is a transcription, not a proposal.

## Command

`command` is the downloader binary. `args` is an **array**; the submitted URL is
its own final element and is never concatenated into any other string.

### Shared by both modes

```text
--parse-metadata  %(album|Singles)s:%(meta_album)s
--parse-metadata  %(artist,uploader)s:%(meta_album_artist)s
--embed-metadata
--embed-thumbnail
--convert-thumbnails jpg
-P                /out
-o                %(artist,uploader)s/%(album|Singles)s/%(track,title)s.%(ext)s
--write-auto-subs
--sub-langs       .*-orig
```

The `-o` template and the first `--parse-metadata` template are **the same
expression**. They must change together or not at all: that identity is what
guarantees the album tag and the album folder cannot disagree.

### Music mode

```text
-x  --audio-format mp3  --audio-quality 0
--convert-subs lrc
--exec  after_move:<constant lyrics-sidecar rename snippet>
```

The rename snippet strips the language infix the downloader always inserts, so
the companion file ends up at `<base>.lrc` — the exact basename Plex and
Jellyfin match on for lyrics.

### Music-video mode

```text
-f  bv*[vcodec^=avc1]+ba[acodec^=mp4a]/bv*+ba/b
--merge-output-format mp4
--convert-subs srt
--exec  after_move:<constant subtitle-rename snippet>
```

Here the snippet rewrites the language segment to a bare 2-letter code, which is
what both media servers parse for video subtitles.

> **Security note.** The two `--exec` values are the only shell in this contract.
> They are **constants authored here**, contain no user input, and receive the
> file path through the downloader's own quoted substitution. The submitted URL
> never appears in them. A unit test asserts the assembled `args` contains the
> URL as exactly one element, and that no element embeds it inside a larger
> string.

### Scope, appended per classification

| Scope | Extra args |
|---|---|
| `single` | `--no-playlist` |
| `playlist` | `--yes-playlist -i --match-filter "duration >= 90" --download-archive /out/.ytdlp-archive.txt` |
| `channel` | `-i --match-filter "duration >= 90" --download-archive /out/.ytdlp-archive.txt --sleep-requests 1 --sleep-interval 3 --max-sleep-interval 8`, with the URL normalised to the channel's videos tab |

The duration bound is **one-sided by design**: it excludes clips, never long
compilations (FR-013).

## Pod spec

| Field | Value | Why |
|---|---|---|
| `image` | operator config, **pinned tag** | Domain Constraint; a floating tag breaks silently |
| `restartPolicy` | `Never` | FR-021 |
| `backoffLimit` | `0` | A retry would repeat a partially completed run |
| `activeDeadlineSeconds` | finite, from config | FR-020 — nothing runs forever |
| `ttlSecondsAfterFinished` | set | Workers sweep themselves; failures survive in the store |
| `runAsUser` / `runAsGroup` | the library's uid/gid | FR-022 — no root-owned files |
| `env` | `XDG_CACHE_HOME=/tmp` | No writable home directory in the pod |
| `resources` | requests + limits | One download must not starve the node |
| `volumeMounts` | exactly ONE library, at `/out` | Makes FR-006 structural: the other library is not mounted, so it cannot be written |
| `automountServiceAccountToken` | `false` | A worker never needs cluster access |

## RBAC the server needs

A namespaced `Role` bound to the `synodl` ServiceAccount. Never a `ClusterRole`.

| Resource | Verbs |
|---|---|
| `batch/jobs` | `create`, `get`, `list`, `delete` |
| `pods` | `get`, `list` |

Explicitly NOT granted: `secrets`, `pods/exec`, `pods/attach`, anything
cluster-scoped, anything outside SynoDL's own namespace.

## Mock orchestrator (`cmd/synok8s`)

Implements only what the client calls — create Job, list by label selector,
delete Job — plus control endpoints so e2e can drive the lifecycle
deterministically:

| Endpoint | Effect |
|---|---|
| `POST /__mock/jobs/{name}/start` | `active = 1` → state `started` |
| `POST /__mock/jobs/{name}/succeed` | condition `Complete` → `completed` |
| `POST /__mock/jobs/{name}/fail` | condition `Failed` → `failed` |
| `POST /__mock/jobs/{name}/deadline` | `DeadlineExceeded` → `failed` |
| `POST /__mock/jobs/{name}/vanish` | drops it from LIST with no terminal condition |
| `POST /__mock/reset` | clears all state between tests |

It **never downloads anything**. The `vanish` control exists specifically to test
the rule that a disappeared Job is never reported as completed.

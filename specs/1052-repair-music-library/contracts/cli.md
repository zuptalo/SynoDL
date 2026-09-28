# Contract: command line

## `scripts/music-repair.sh` (runs on the operator's machine; needs `kubectl` + the `synodl` context)

| Command | Effect |
|---|---|
| `plan` | Creates the plan Job. Writes `.repair/plan-<id>.json` + `.md`. **Changes nothing else.** Prints the id and the totals. |
| `apply <id>` | Creates the apply Job for that plan. Refuses without an id, refuses if `plan-<id>.json` is missing or its `library_fingerprint` no longer matches enough to proceed (steps that no longer match are skipped, not forced). |
| `status [<id>]` | Prints the last lines of the Job log and the results file totals. |
| `restore <id>` | Creates a Job that reverses `move`/`trash` recorded in that journal. |

Env: `NAMESPACE` (default `synodl`), `CONTEXT` (default the current one).
Exit codes: `0` ok · `2` bad arguments · `3` Job failed · `4` refused (no plan / not enough space / stale).

## `python3 -m music_repair` (runs in the Job; also runnable locally against a mounted copy)

```
python3 -m music_repair plan    --library DIR [--repair-dir DIR] [--no-lookup] [--limit N]
python3 -m music_repair apply   --library DIR --plan ID [--repair-dir DIR]
python3 -m music_repair restore --library DIR --plan ID [--repair-dir DIR]
```

- `--repair-dir` defaults to `<library>/.repair`; overridable so a **local dry run never writes to the library**.
- `--no-lookup` skips all network; songs are `not_looked_up`. Useful for an offline plan.
- `--limit N` plans only the first N artist folders (testing).
- Exit codes as above. Output: one line per phase with counts; relative paths only; never a
  tag description, a query string, or an environment variable.

## Outbound (both commands)

Only HTTPS to: `musicbrainz.org`, `coverartarchive.org`, `archive.org` (redirect target),
`itunes.apple.com`, `mzstatic.com`, `api.deezer.com`, `dzcdn.net`. Suffix match on a dot boundary
(`evilarchive.org` ≠ `archive.org`); every redirect hop is re-validated; max 3 hops; 15 s
timeout; response body cap 8 MB. MusicBrainz: ≤ 1 request/second, `User-Agent:
SynoDL-music-repair/<version> (+https://github.com/zuptalo/synodl)`.

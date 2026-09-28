# Contract: `.repair/plan-<id>.json`

```json
{
  "version": 1,
  "id": "20260928T201500Z-a1b2c3",
  "created": "2026-09-28T20:15:00Z",
  "library": "/library",
  "totals": {
    "tracks": 5766, "songs": 3067, "duplicates_to_trash": 1770, "moves": 0, "retags": 0,
    "covers": 0, "playlists": 0, "conversions": 2, "conflicts": 0, "unidentified": 0,
    "bytes_reclaimed": 0, "bytes_needed": 0,
    "matched": 0, "no_match": 0, "not_looked_up": 0
  },
  "actions": [
    {"id": "a000001", "kind": "trash", "reason": "duplicate of a000431 (video 5qm8PH4xAss)",
     "src": "50 Cent/Dance Music 2000 to 2026 …/50 Cent - In Da Club (Official Music Video).mp3",
     "dst": ".trash/<id>/50 Cent/Dance Music 2000 to 2026 …/50 Cent - In Da Club ….mp3",
     "size": 7019710, "mtime_ns": 1759000000000000000}
  ],
  "unidentified": ["…relpaths with no video id…"]
}
```

Rules:
- `actions` are ordered so that every step's preconditions are met by earlier steps
  (`convert` → `retag` → `move` → `cover` → `playlist` → `trash` / `orphan_nfo` last).
- `src`/`dst` are library-relative, never absolute and never containing `..`; apply
  re-validates this (`safe_rel`) before touching anything.
- `size` + `mtime_ns` are the staleness guard for the source file, checked once per source file at its first action; later actions on the same file (retag, move, cover) follow it via the journal and are not re-checked.
- No tag descriptions, no URLs other than the cover URL of an action of kind `cover`.
- The `.md` rendering has: totals table, per-kind counts, top 25 biggest reclaim, every
  `conflict`, every `unidentified`, the no-match list with the nearest candidate's length,
  and a sample of 20 moves per kind.

# Contract: worker → server events

The tool (`python3 -m music_repair`) keeps printing its human-readable lines. In
addition it prints **event lines** to stdout: `@@synodl ` followed by one JSON object on
the same line. The server keeps only these and ignores every other line.

- One object per line: ≤ 4096 bytes for `progress`, ≤ 16384 bytes for `result` (20 example paths of ≤ 200 characters plus reasons is about 9 KB). Written with `ensure_ascii=False`, no newlines inside.
- Emitted through one function (`events.emit`) so the shape and the bounds are enforced in
  one place on the sending side too; the receiving side re-checks everything.

## `progress`

```json
@@synodl {"event":"progress","phase":"lookup","done":1275,"total":4017}
```
`phase` ∈ `scan`, `lookup`, `plan`, `apply`, `restore`. Emitted at each phase boundary and
periodically (every 25 lookups, every 200 apply steps). `total` may be `0` when unknown.

## `result` (always the LAST event a run prints)

Check:
```json
@@synodl {"event":"result","kind":"check","ok":true,"planId":"20260929T071341Z-73c844",
 "planFile":".repair/plan-20260929T071341Z-73c844.md",
 "check":{"tracks":5795,"songs":4017,"duplicates":1774,"moves":3970,"retags":4017,"covers":886,
   "playlists":88,"conflicts":0,"nameClashes":52,"matched":1828,"noMatch":2186,"notLookedUp":3,
   "toSingles":2917,"albumKnown":886,"orphanNfo":3484,"bytesReclaimed":13368173185,
   "bytesNeeded":548758400,"freeBytes":4600000000000,
   "leftAlone":{"total":6,"byReason":[{"reason":"no video id","count":4},{"reason":"an mp3 of that name exists","count":2}],
                "examples":[{"path":"Avaria/Singles/Hold Me Down.mp3","reason":"no video id"}]}}}
```
Apply:
```json
@@synodl {"event":"result","kind":"apply","ok":true,"planId":"…",
 "apply":{"done":15976,"skipped":155,"failed":0,"alreadyDone":0,
   "skippedByReason":[{"reason":"no cover art","count":139}],"failedExamples":[]}}
```
`ok` is `false` when the tool exits non-zero for a failed step (`reason: "failed_steps"`).
Undo (`kind:"undo"`, tool command `restore`):
```json
@@synodl {"event":"result","kind":"undo","ok":true,"planId":"…","undo":{"restored":15976,"skipped":0,"skippedExamples":[]}}
```
Refusal (the tool declined, exit 4):
```json
@@synodl {"event":"result","kind":"apply","ok":false,"reason":"locked"}
```
`reason` ∈ `locked`, `no_plan`, `no_space`, `rejected`, `failed_steps`.

## Receiving rules (server)

1. Read at most 256 KiB, tailing 200 lines. Consider at most 64 event lines.
2. Drop a line that does not start with the prefix, is longer than its cap (4 KiB progress, 16 KiB result), or is not a JSON object.
3. Decode into the fixed types of `data-model.md`; ignore unknown fields; unknown `event`,
   `phase` or `reason` values are dropped (not passed through).
4. Clamp integers to `[0, 10¹²]`; strip control characters and cap string length (paths
   200, reasons/notes 120); cap arrays (12 reasons, 20 check examples, 10 apply/undo examples).
5. `planId` must match `^\d{8}T\d{6}Z-[0-9a-f]{6}$` or is dropped.
6. Only the last valid `result` and the last valid `progress` are used.
7. Never log any of it; never forward the raw line.

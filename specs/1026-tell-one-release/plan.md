# Implementation Plan: Tell one release from another by the file it makes

**Spec**: [spec.md](./spec.md) · **Branch**: `feat/1026-tell-one-release`

## Summary

Spec 1025 tells releases apart by two tokens read from the file name: the
resolution and the release group. Checked against the live source, ZarFilm defeats
that comparison — it renames everything it serves with its own suffix, so four
different encoders collapse onto one group, and it labels every movie option with
its own name as the encoder.

Compare the **file each option produces** instead. The download link names it, the
NAS saves it under that name, so the two sides can be compared directly rather
than through tokens the site has overwritten. Tokens stay as the fallback for
sources that do not name their files.

## Research (live source, 2026-09-04)

One series, one season, six qualities. What the site says, and what it actually
serves:

| Quality says | File it serves ends |
|---|---|
| `… - NHTFS` | `…H.264.NHTFS-ZarFilm.mkv` |
| `… - ZarFilm` | `…1080p.-ZarFilm.mkv` |
| `… - PSA` | `…x265.PSA-ZarFilm.mkv` |
| `… - Pahe` | `…x264.Pahe-ZarFilm.mkv` |
| `… - RMTeam` | `…x264.RMT-ZarFilm.mkv` |
| `… - ZarFilm` (dubbed) | `…Dubbed.ZarFilm.mkv` |

Run through the shipped matcher, the first five all yield the group `zarfilm` and
the last yields nothing. A movie is worse still: all six of its options are
labelled with the same encoder, and its files end `.ZarFilm` with no separator, so
none of them identifies anything.

Two conclusions, both load-bearing:

1. **The group cannot discriminate here** — not because the parser is wrong, but
   because the site overwrote the thing the parser reads. The label and the file
   do not even agree (`RMTeam` against `RMT`), so comparing the label to the file
   would not rescue it either.
2. **The file name can.** Every row above serves a distinct file, including the
   two the tokens cannot separate (`Dubbed` against `x264` at one resolution).

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)
**Primary Dependencies**: none added
**Storage**: none — the reduced identity lives in the existing evidence cache
**Testing**: `go test ./...` (pure identity + fake `syno.Client`); vitest; Playwright
**Project Type**: web (Vue PWA + Go proxy)
**Performance Goals**: no additional NAS call and no additional source request
**Constraints**: the file name must not reach the client, a log, or an error

## Constitution Check

| Principle | Verdict |
|---|---|
| I — Spec-first | Spec written and researched before this plan. |
| II — TDD | The identity function's tests land first, against the real names captured above. |
| III — Credential boundary | Unchanged. No allowlist change, no NAS call added. The option's file name is source data, not NAS content, and is excluded from the wire by its tag; NAS names still never leave the server. |
| IV — Stateless where it can be | Derived, in-memory, rebuilt on restart. |
| V — Release-note commit subjects | `fix(discover): tell ZarFilm's versions apart so the one you have is the one marked` |
| VI — Ionic-first UI | No UI change beyond a season option showing its encoder in the existing row. |

No violations.

## Design

### 1. Release identity — `server/internal/library/release.go`

```go
// ReleaseKey reduces a file name to what makes it THIS release.
func ReleaseKey(name string) string
```

Lower-cased, extension dropped, every run of non-alphanumerics collapsed, and the
**episode token removed** so `S01E01` and `S01E05` of the same release agree —
without which a season the user only partly downloaded would fail to identify
itself (FR-003, spec acceptance 3).

`Release` gains a `Key` field; `ReleaseOf` fills it always, even when it cannot
find a resolution or a group. The existing `ok` result keeps its meaning for the
token path, so nothing about spec 1025's behaviour shifts for sources where it
already works.

### 2. The option says what it produces — `server/internal/source/source.go`

```go
// ReleaseName is the file this option would produce. Server-internal.
ReleaseName string `json:"-"`
```

`json:"-"` is the point: it cannot be serialised by accident, so FR-002 is
enforced by the type rather than by remembering (the same trick that would have
prevented a sid reaching a log earlier in this codebase's history).

### 3. Matching — `server/internal/api/library.go`

`evidenceRec` gains `keys map[int][]string`, filled from the same file names
`folderEvidence` already reads.

```
option names a file ─ yes ─▶ compare keys. Match → owned. No match → NOT owned.
                    └─ no ──▶ compare resolution + group, exactly as today.
```

The mismatch branch is deliberately **definitive** (FR-004). Falling back to
tokens after a filename mismatch would reintroduce the over-marking this exists to
remove: on ZarFilm every token comparison succeeds, so a fallback would mark every
option again.

### 4. ZarFilm supplies the names — `server/internal/source/providers/`

- Movies: the row's own download URL.
- Series: the first episode URL of that season's quality — the episode number is
  reduced out of the key, so any episode identifies the release.
- A paywalled row has no link and so names nothing, which is correct: it is an
  upsell, not a release.

### 5. Series encoders — `server/internal/source/providers/zarfilm_parse.go`

A season's quality reads `WEB-DL 1080p x265 10bit - PSA`: the encoder is the
segment after the final ` - `. Extracted into `QualityOption.Encoder` so a season
row reads like a movie row (FR-008), and left empty when there is no such segment
rather than guessing (FR-009).

This is what the user asked for directly. It improves the row and feeds the token
fallback, but it is **not** what makes matching work here — the research above is
why.

## Project Structure

```
server/internal/library/release.go        # + ReleaseKey, Release.Key
server/internal/source/source.go          # + QualityOption.ReleaseName (json:"-")
server/internal/api/library.go            # keys in evidence; key-first matching
server/internal/source/providers/
├── zarfilm_parse.go                      # season encoder from the quality text
└── zarfilm.go                            # supply ReleaseName for both shapes
server/internal/synomock/sources.go       # fake files named like real releases
e2e/stateful/                             # two options a token cannot separate
```

## Phases

**Phase 1** — `ReleaseKey` against the captured real names.
**Phase 2** — carry the name; key-first matching.
**Phase 3** — ZarFilm supplies names; season encoders.
**Phase 4** — gates, roadmap, PR.

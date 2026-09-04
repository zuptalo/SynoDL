# Implementation Plan: Mark the version you actually downloaded

**Spec**: [spec.md](./spec.md) · **Branch**: `feat/1025-mark-version-you`

## Summary

Ownership is resolved per season, so every option for a season on disk is stamped
"Have it" — three of four falsely. The files themselves say which release they
came from: SynoDL saves a download under the name it arrived with, and those names
carry a resolution and a release group. `folderEvidence` already reads those names
to work out which episodes are present; it just throws the rest away.

Keep the release tokens, compare them against each download option server-side,
and mark only the option that matches. Then group the options by season in the
sheet so the user reaches the season they want without scrolling past the ones
they already have.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)
**Primary Dependencies**: none added
**Storage**: none — tokens live in the existing in-memory evidence cache
**Testing**: `go test ./...` (pure matching + fake `syno.Client`); vitest; Playwright
**Project Type**: web (Vue PWA + Go proxy)
**Performance Goals**: no additional NAS call — the file names are already listed
**Constraints**: no new DSM API, no new persistence, nothing about the NAS in logs
or sent to the client beyond a per-option boolean

## Constitution Check

| Principle | Verdict |
|---|---|
| I — Spec-first | Spec written before this plan. |
| II — TDD | The matcher's tests land before the matcher; the accordion's before the accordion. |
| III — Credential boundary | **Narrowed, not widened.** No new NAS call, no allowlist change. Matching moves server-side so file names and release tokens never cross to the client — less about the NAS leaves the server than today. |
| IV — Stateless where it can be | Tokens are derived, cached in memory beside the season presence they came from, and rebuilt on restart. |
| V — Release-note commit subjects | `fix(discover): mark the version you actually downloaded, not every version` |
| VI — Ionic-first UI | The accordion is `ion-accordion-group`/`ion-accordion`, not a hand-rolled disclosure. |

No violations. A `/speckit-checklist` run is not required: the boundary is
approached but moves inward, and the Credential-Safety Impact section records why.

## Design

### 1. Reading a release off a file name — `server/internal/library/release.go`

A pure function, so it carries the package's coverage floor and needs no NAS:

```go
// Release is how one copy was encoded.
type Release struct{ Resolution, Group string }

func ReleaseOf(name string) (Release, bool)
```

- **Resolution** from the usual tokens (`2160p`, `1080p`, `720p`, `480p`), plus
  `4K`/`UHD` normalised onto `2160p`.
- **Group** from the trailing `-GROUP` before the extension, or a bracketed
  `[GROUP]`. Both are how release names are actually written.
- ok is false unless BOTH are found. A half-identified file identifies nothing
  (FR-002), and returning a partial match would be the resolution-only guess the
  user explicitly rejected.

### 2. Keeping them — `server/internal/api/library.go`

`evidenceRec` gains `releases map[int][]library.Release` keyed by season, with
season 0 meaning "the title folder itself" so a movie is carried by the same
field. `folderEvidence` records `ReleaseOf(name)` for each video it already
inspects. No extra NAS call: the names are in hand.

### 3. Matching, server-side — `server/internal/api/library.go`

`QualityOption` gains `Owned bool \`json:"owned,omitempty"\``, set by the handler
on the way out — the same decoration pattern `Ownership` and `Seasons` use, and
for the same reason: a driver must not be able to claim it.

An option matches when a release on disk for that option's season agrees on
resolution AND group, compared case-insensitively and ignoring separators (a file
says `x265-Silence`, an option says `Silence`). No match leaves every option for
that season unmarked, and the season's own presence is untouched (FR-004).

Doing this on the server rather than the client means the release tokens never
cross the boundary at all — the client learns only "you already have this one",
which is less about the NAS than the episode numbers it already receives.

### 4. Season groups — `src/components/SourceTitleModal.vue`

Options that carry a season are grouped into an `ion-accordion-group` with
`:multiple="false"`, which gives single-open behaviour for free (FR-009).

- The initially expanded value is the first season **not** present on the NAS; if
  every season is present, none (FR-008).
- A collapsed header states the season, whether it is on the NAS, and how many
  options it holds (FR-010).
- Options with no season bypass the accordion entirely and render as today
  (FR-011) — a movie has one group, and a group of one is a layer for nothing.
- Expansion state is presentation only: `selected` is untouched, so sending is
  unchanged (FR-012).

### 5. Rows that fit — `src/components/SourceTitleModal.vue` styles

The row is a flex line whose badge is clipped and whose episode list is cut off.
The title line wraps with the badge kept whole (`flex-wrap` and a
non-shrinking badge), and the episode list wraps rather than being truncated.
Verified at 360px, which is narrower than the reporting device.

## Project Structure

```
server/internal/library/
├── release.go            # NEW — ReleaseOf
└── release_test.go       # NEW

server/internal/api/
├── library.go            # keep tokens; match options
└── library_test.go       # matching against a fake client

server/internal/source/source.go   # + QualityOption.Owned
server/internal/synomock/          # seed files with real release names

src/services/api.ts                # + QualityOption.owned
src/components/SourceTitleModal.vue # accordion + responsive rows
e2e/stateful/                      # only the matching option is marked; accordion behaviour
```

## Phases

**Phase 1** — `ReleaseOf` and its tests.
**Phase 2** — keep tokens in the evidence; match; mark. Mock seeds real names.
**Phase 3** — accordion.
**Phase 4** — responsive rows.
**Phase 5** — gates, roadmap, PR.

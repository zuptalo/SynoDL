# Implementation Plan: Give YouTube downloads their own button

**Branch**: `feat/1033-give-youtube-downloads` | **Date**: 2026-09-07 | **Spec**: [spec.md](./spec.md)

## Summary

Client-only. The endpoint, the host allowlist and the argv handling from spec
0012 are untouched; this moves where a user expresses the request.

A third action joins the existing `+` menu — which is already an `ion-fab-list`
with two — and opens a sheet containing a link field and a two-way mode choice
and nothing else. The mode selector is removed from the general new-task sheet,
which instead points at the new button when it notices a YouTube link.

## Technical Context

**Language/Version**: TypeScript 5 / Vue 3 + Ionic. No server change.

**Primary Dependencies**: None added.

**Storage**: None.

**Testing**: Playwright for the new sheet and for the general sheet no longer
offering a mode. The submit path itself is already covered by spec 0012's tests.

**Constraints**: The `+` action must not appear where the server cannot run
downloads (FR-002), which the client already knows from the 503 the feed returns.

## Constitution Check

| Rule | How this satisfies it |
|---|---|
| Ionic-first UI | A third `ion-fab-button` inside the existing `ion-fab-list`, and a standard `ion-modal`. No new widget, no bespoke pattern. |
| Custodial state | Nothing stored. |
| Worker inputs allowlisted | Unchanged — validation stays server-side. |
| TDD | e2e precedes the component work; the pure submit logic is already covered. |

**Gate result**: PASS. Complexity Tracking empty.

## Project Structure

```text
src/
├── components/
│   ├── YoutubeDownloadModal.vue   # NEW — link + mode, nothing else
│   └── NewTaskModal.vue           # mode selector removed, nudge added
└── views/tabs/TasksPage.vue       # third FAB action + the new sheet
e2e/stateful/
└── ytdl-fab.spec.ts               # NEW
```

**Structure Decision**: A separate component rather than a mode flag on
`NewTaskModal`. The whole defect is that one sheet was trying to be two things;
expressing that as a conditional inside the same component would preserve the
problem in the code while hiding it in the UI.

## Design notes

- **No preselected mode.** FR-004 asks for an explicit choice, so Add stays
  disabled until one is made. Defaulting to Music would be convenient and would
  also be exactly the class of bug this spec exists to remove: a default that
  quietly decides something the user came here to decide.
- **The nudge is a note, not a shortcut.** The general sheet tells the user the
  dedicated button exists; it does not offer to do it for them, because that
  would re-introduce two ways to do one thing.

# Implementation Plan: Make YouTube downloads readable in the task list

**Branch**: `feat/1034-make-youtube-downloads` | **Date**: 2026-09-07 | **Spec**: [spec.md](./spec.md)

## Summary

One bounded HTTP call when a request is accepted, three annotations on the Job
it creates, and a narrow image proxy so the browser never talks to Google.
Nothing is stored, nothing about progress is learned, and a failed lookup
changes nothing about the download.

## Technical Context

**Language/Version**: Go 1.26 (server), TypeScript 5 / Vue 3 + Ionic (client)

**Primary Dependencies**: None added. The lookup is `net/http` +
`encoding/json`; the proxy reuses the shape of the existing poster proxy.

**Storage**: None. Description rides on the Job as annotations, exactly as the
submitted link and submitter already do (spec 0012).

**Testing**: `httptest` for the lookup, including timeout and garbage-response
paths; handler tests with a fake; Playwright for the rendered row.

**Performance Goals**: Submission must feel unchanged (SC-003), so the lookup
carries a short deadline and its failure is a non-event.

**Constraints**: The row's height must not change with title length (FR-011).

## Constitution Check

| Rule | How this satisfies it |
|---|---|
| Worker inputs allowlisted | The link passes the existing host allowlist BEFORE any lookup, so this cannot be used to make the server fetch an arbitrary address. |
| One store, one volume | Nothing written. Annotations are the orchestrator's state, like the URL already there. |
| Job state owned by the orchestrator | Unchanged — this adds description, never lifecycle. |
| Credentials never leak | The lookup sends no credential, session, or user identity. The proxy adds no new secret. |
| Ionic-first UI | The row keeps its existing structure; the thumbnail slots where the placeholder icon already is. |
| TDD | Lookup and proxy are tested before use, including their failure paths. |

**Gate result**: PASS. One entry in Complexity Tracking.

## Project Structure

```text
server/internal/
├── ytdl/
│   ├── meta.go / meta_test.go     # NEW — bounded oEmbed lookup
│   └── job.go                     # + title/uploader/thumbnail annotations
└── api/
    ├── ytdl_handlers.go           # look up on submit; serve the view fields
    └── ytdl_thumb.go              # NEW — narrow artwork proxy
src/
├── services/api.ts                # YtdlDownload gains the three fields
└── components/YtdlItem.vue        # title, uploader, artwork
```

## Design decisions

**The artwork proxy gets its own host rule.** The obvious move is to reuse
`/v1/source/image`, but its allowlist is assembled from download-source drivers
plus operator-configured mirrors — and YouTube is neither. Sharing one list
would mean an operator editing a source could change what this feature may
fetch, and a change here could widen what source images may come from. Two
concerns, two lists.

**The lookup is fire-and-wait-briefly, not fire-and-forget.** Doing it
asynchronously would need somewhere to put the result later, which means either
state or a second write to the Job. A short bounded call before creating the Job
keeps the whole thing one transaction and one code path, and the deadline is
what protects submission latency.

**A failed lookup is silent.** It is not an error the user needs; the row simply
falls back to the link, which is exactly what it shows today.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| A second image proxy endpoint | Artwork must be fetched server-side (FR-008) and its host list must not mix with the download sources' (FR-009). | Widening `/v1/source/image` couples two unrelated allowlists — an operator's source edit would change what this feature can fetch. Letting the browser fetch directly leaks the viewer's IP to Google and abandons the reason the poster proxy exists. |

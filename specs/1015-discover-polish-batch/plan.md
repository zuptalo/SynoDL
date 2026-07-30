# Implementation Plan: Discover polish batch

**Branch**: `feat/1015-discover-polish-batch` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/1015-discover-polish-batch/spec.md`

## Summary

A running batch of small Discover refinements shipped together in one PR/release. Current contents:

- **US1 — Remove the live-streamable filter**: SynoDL won't offer live streaming, so the "Streamable
  only" filter can only mislead. Removed end to end — the toggle + active-filter chip (client), the
  `stream` field on the client filter model and the server `SearchFilters.Stream` / request JSON /
  `buildParams` plumbing. No results-behaviour change beyond dropping the dead filter.

## Technical Context

**Language/Version**: TypeScript / Vue 3 + Ionic (client); Go 1.26 (server)
**Primary Dependencies**: Ionic Vue; stdlib net/http
**Storage**: none — no persisted state changes
**Testing**: vitest unit; `go test ./...` (fake source client)
**Target Platform**: PWA; k3s via Keel
**Project Type**: web application
**Constraints**: stateless credential-free proxy; source allowlist unchanged (removing a param only)
**Scale/Scope**: 4 client files, 3 server files, 1 test updated

## Constitution Check

- **I. Spec-Driven** — ✅ spec `1015` drives it.
- **III. Custodial State & Credential Safety** — ✅ removes a request param; no state, no credentials,
  no new/widened source API. Checklist gate not triggered.
- **TDD** — ✅ the affected server table test is updated to drop the `stream` assertion; the rest is
  deletion of UI + plumbing (no new logic to test).
- **V. Release-note subject** — ✅ user-facing plain-language `feat(browser)` subject.

No violations → no Complexity Tracking.

## Project Structure

Docs: `specs/1015-discover-polish-batch/{spec,plan,tasks}.md` + `checklists/requirements.md`.
Research/data-model/contracts: N/A (removal, no new decisions/entities/contracts).

Source touched:
```text
src/components/SourceFilterSheet.vue   # drop stream ref/watch/apply/clear/toggle
src/views/tabs/BrowserPage.vue         # drop the stream chip
src/composables/useSourceCatalog.ts    # drop f.stream from hasFilters/searchIneffective
src/services/api.ts                    # drop stream? from SourceSearchFilters
server/internal/source/source.go       # drop SearchFilters.Stream
server/internal/source/providers/thirtynama.go        # drop buildParams stream
server/internal/api/source_handlers.go # drop stream req field + mapping
server/internal/source/providers/thirtynama_test.go   # drop stream from the facets test
```

## Complexity Tracking

No constitution violations — section intentionally empty.

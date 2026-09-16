# Tasks: The last title's cast stops appearing on the next one

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED, and for a `2001+` fix the constitution is specific: the work
BEGINS with a failing regression test that reproduces the bug.

No plan/research artifacts: this changes when the sheet renders what it already
has. No driver, endpoint, cache, stored value or outbound call is touched, so
there is nothing to design and nothing for a checklist to review.

- [x] T001 Write the failing regression in `e2e/stateful/discover-credits.spec.ts`: with the source made slow, open one title, close it, open another, and assert the first title's people are not on screen while the spinner is up — confirmed failing against the shipped build (`44 × locator resolved to <section class="credits">`)
- [x] T002 Clear `detailMeta` in the open watcher in `src/components/SourceTitleModal.vue`, alongside the other per-title state it already resets (FR-001, FR-003)
- [x] T003 Move `<source-credits>` inside the non-loading branch, between the synopsis and the download options, in `src/components/SourceTitleModal.vue` (FR-002, FR-004)
- [x] T004 Drop the external-link icon from the person tile in `src/components/SourceCredits.vue`, keeping the aria-label that says where it goes (FR-005)
- [x] T005 Flip the layout assertion in `e2e/stateful/discover-credits.spec.ts` and assert the icon is gone while the tile is still an announced link
- [x] T006 Run the full gate: `npm run build`, `npm run test:unit:coverage`, `go build/vet/test`, `npm run test:e2e`
- [x] T007 Set `**Status**: in-review` and run `make roadmap`

## Why the first version of T001 did not count

It passed against the buggy build. `toBeHidden()` was satisfied by the instant
before the modal had even opened, so it asserted nothing. It only became a
regression test once it waits for the sheet to be demonstrably IN its loading
state first — which is the difference between a test that fails for the right
reason and one that never fails at all.

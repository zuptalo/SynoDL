# Tasks: ZarFilm browses through the site's advanced search

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

No plan/research artifacts: the driver's outbound surface is unchanged (same
site, one extra path on it), no stored value or credential handling changes.

- [x] T001 Investigate live: every advanced-search field, its values, what composes, what is dropped (one-ended ranges), what is broken (4K), how the dialog is fetched; capture vocabularies
- [x] T002 Fixture `testdata/zarfilm/advanced_search_form.json` from the captured dialog; `parseAdvancedForm` + test
- [x] T003 `Search`: browse = advanced search with every field; text = `s` only (FR-001–FR-003), request-level tests
- [x] T004 `Parameters`: dialog-backed facets, genre slugs from the archive routes, ISO codes for languages (`zarfilm_lang.go`), shared score slugs, canonical sorts; degrade path (FR-004, FR-005)
- [x] T005 Fake site (`synomock`) serves the dialog and honours the advanced parameters; e2e expectations updated (FR-006)
- [x] T006 Gate: `go test` (both tags), `npm run build`, e2e
- [x] T007 Merge; confirm in production against https://zhomis.info
- [x] T008 Set `**Status**: shipped` and run `make roadmap`

# Tasks: Filters that work across every source

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

No plan/research artifacts: no endpoint, stored value, outbound host or
credential path changes — this is identity (slugs) on declared facets, one more
facet through the existing translator, and one merge rule.

- [x] T001 Map the fan-out, the facet join, the translator, both drivers' vocabularies (live), and why the combined sheet was empty
- [x] T002 30nama: type/language/score-under-5 identities (FR-001), with tests
- [x] T003 Type joins the translated facets (FR-002), with a test
- [x] T004 Persian folding in `normalizeFacet`; slugged representative in `IntersectParameters` (FR-003, FR-004), with tests
- [x] T005 `MergeByScore` for the IMDb ordering in `SearchAll` (FR-005), with tests
- [x] T006 Client labels resolve via slug first (FR-004), with tests
- [x] T007 `providers/combined_test.go`: real fixtures intersect to a non-empty sheet (FR-006)
- [x] T008 Fake 30nama declares languages/countries; fake ZarFilm's countries in Persian; e2e: combined type/language/country offered and applied to both sources; IMDb ordering exact across sources
- [x] T009 Gate: `go test` (both tags), `npm run build`, unit coverage, e2e
- [ ] T010 Merge; confirm in production with both sources
- [ ] T011 Set `**Status**: shipped` and run `make roadmap`

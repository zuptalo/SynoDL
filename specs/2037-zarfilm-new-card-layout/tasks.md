# Tasks: ZarFilm's catalog shows titles again after the site's redesign

**Input**: [spec.md](./spec.md)

- [x] T001 Diagnose in production: `/v1/source/search` for the ZarFilm source returns 200 / 1338 pages / 0 items; inspect the live archive, series archive, a movie page and a series page in a signed-in browser — only the listing card changed
- [x] T002 Fixture `testdata/zarfilm/archive_zf_cards.html` from the live markup; failing test `TestParseListingReadsTheCurrentCards` (`parsed 0 items`)
- [x] T003 `parseZfCard` alongside the old card in `parseListing` (FR-001, FR-002)
- [x] T004 Gate: `go test` (both tags), `npm run build`
- [x] T005 Merge; confirm in production that Discover lists ZarFilm titles
- [x] T006 Set `**Status**: shipped` and run `make roadmap`

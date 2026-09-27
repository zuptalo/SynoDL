# Tasks: ZarFilm is reached only at the address the operator gives

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

No plan/research artifacts: the outbound surface only NARROWS (a fixed host is
removed; the operator's address was already allowlisted per source by 0009/1020),
no stored column changes, and credential handling is unchanged. That is also why
no `checklist` run: nothing widens the credential boundary.

- [x] T001 Map how a source's address flows (driver, allowlists, image proxy, setup API, form, mocks, tests); confirm no ZarFilm source in production
- [x] T002 `source.AddressRequirer` / `source.RequiresAddress` capability
- [x] T003 Driver: drop `zarHost`/`zarBase`/`DefaultAltBase`; `bases()` from configuration only; `VerifySession` over the configured addresses; links accepted on configured addresses only (FR-001, FR-004)
- [x] T004 API: `resolveBases` requires a main address for such a driver, promotes a lone alternate, stops filling main with the default mirror; `address_required` error; `kindView.addressRequired`; runtime promotion in `sourceRefs`; legacy route refuses (FR-002, FR-003, FR-005, FR-006)
- [x] T005 Tests: no built-in host reachable; no address → unavailable; verification uses the configured address; fixtures served self-linking; API rules against a fake address-requiring driver; production/dev build tag tests (FR-007)
- [x] T006 Form: required "Site address" with guidance; `address_required` message
- [x] T007 Docs: `docs/DOWNLOAD-SOURCES.md`, `docs/UPGRADING.md`
- [x] T008 Gate: `go build/vet/test` (both tags), `npm run build`, unit coverage, e2e
- [ ] T009 Merge; confirm the deploy, and that the add-source form asks for the site address
- [ ] T010 Set `**Status**: shipped` and run `make roadmap`

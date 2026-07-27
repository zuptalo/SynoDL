<!--
Thanks for contributing to SynoDL! Keep the summary focused on user-facing behavior.
See CONTRIBUTING.md for the full workflow.
-->

## What & why

<!-- What does this change do, and why? Link any issue. -->

## Checklist

- [ ] Targets `develop` (not `main`).
- [ ] `npm run build` passes (typecheck + bundle).
- [ ] `npm run test:unit` passes; added/updated unit tests where it made sense.
- [ ] `cd server && go test ./...` passes; added/updated `_test.go` where it made sense.
- [ ] `npm run test:e2e` run if this affects user-facing flows (or N/A).
- [ ] Commits follow Conventional Commits with a scope (e.g. `feat(tasks): …`).

## Credential-safety invariant

<!--
Required if this touches the proxy (anything under server/, the /v1 surface, or
what the client sends it). The server must stay stateless and credential-free.
-->

- [ ] This change stores **no** state on the server, logs **no** credential/sid/URI,
      and adds **no** DSM API outside `server/internal/syno` — **or** it does not
      touch the proxy.
- Notes: <!-- how the proxy stays empty-handed, if relevant -->

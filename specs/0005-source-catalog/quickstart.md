# Quickstart: Download-source catalog

## For the operator/admin (one-time setup)

1. Sign in to SynoDL as **admin**. The feature is **off by default** until you configure a provider.
2. On your own computer, log in to the provider site in a browser **from the same network as the NAS** (so the
   public IP matches — this is required; the provider binds sessions/links to that IP).
3. In the browser DevTools, capture the session material:
   - the **User-Agent** (`navigator.userAgent`),
   - the **`cf_clearance`** cookie,
   - the API auth headers the site's app sends (`c-api-key`, `c-token`, `c-platform`, `c-app-version`) — from
     any XHR request to the provider's API (Network → Fetch/XHR → Request Headers).
4. In SynoDL **Settings → Download source**, choose the provider, set the **movies parent folder** (and TV
   parent for later), paste the captured values, and **Verify & Save**. SynoDL performs a sample search; it
   stores the material (encrypted) only if that succeeds.
5. The **Browser** tab now shows the catalog for all users.

**When it stops working** ("source needs refreshing"): the clearance cookie or the auth token expired (or your
public IP changed). Re-capture the values and paste them again in Settings — one step, everyone's catalog
resumes. Secrets are never shown back to you or anyone.

## For users

1. Open the **Browser** tab → browse or search the catalog; filter by type / quality / genre / language /
   country.
2. Open a movie → pick a quality (size/resolution shown) → **Send to NAS**.
3. SynoDL creates a subfolder named for the movie under the movies parent and adds the download in **Tasks**,
   where it behaves like any other Download Station task. Set a **preferred quality** to skip the picker when a
   title offers it.

*(v1: movies are sendable end-to-end; series & anime are browsable/searchable, with send coming next.)*

## For developers

- Server: `internal/source` (provider abstraction + stdlib HTTP/2 client, host-allowlisted) + a
  `providers/thirtynama.go` driver; migration 0005 + `source_repos.go`; handlers in `source_handlers.go`.
- Tests: provider client + handlers against an `httptest` fake provider and the fake `syno.Client`; store
  round-trip incl. encryption + write-only; redaction assertions; a Playwright happy-path against a mock
  provider. Run the gates: `npm run build`, `cd server && go build/vet/test ./...`, `npm run test:unit`,
  `npm run test:e2e`.
- Nothing runs against a real NAS or the real provider in CI — use the mock provider double, matching the
  mock-DSM parity rule.

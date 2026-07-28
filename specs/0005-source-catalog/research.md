# Phase 0 Research: Download-source catalog

All items below were resolved by **live end-to-end testing** against the first provider (a movie/TV site)
before planning, plus review of the existing SynoDL server. No open NEEDS CLARIFICATION remain.

## R1 — Bypassing the provider's bot protection from the server

- **Decision**: Call the provider with Go's **stdlib `net/http` over HTTP/2** (`Transport{ForceAttemptHTTP2:
  true}`), sending the stored **User-Agent** + browser-like headers (`Sec-Ch-Ua*`, `Sec-Fetch-*`) and the
  stored **`cf_clearance`** cookie. XHR-style calls additionally send `Accept: application/json`,
  `Sec-Fetch-Mode: cors`, `Origin`/`Referer` of the site.
- **Rationale**: Verified empirically — HTTP/1.1 gets a Cloudflare `challenge` (403); **HTTP/2 with the
  clearance cookie + matching UA returns 200** on the site and the API subdomain (clearance carries across
  subdomains). Works from the same public IP the cookie was minted on.
- **Alternatives rejected**: headless browser / FlareSolverr (heavy, violates the pure-Go/one-image ethos);
  uTLS/JA3 spoofing (third-party dep, unnecessary — stdlib h2 sufficed).

## R2 — Authenticating to the provider API

- **Decision**: The API authenticates via **custom headers**, not cookies: `c-api-key` (app key), `c-token`
  (per-user login), plus `c-platform` / `c-app-version` / `c-useragent`. Store these as part of the session
  material and attach to every API call.
- **Rationale**: The exported cookie set contained no session cookie; the API returned origin-`404` until the
  `c-*` headers were supplied, then returned real JSON. So bot-protection (cookie) and API-auth (headers) are
  **two independent layers**.
- **Implication**: Two independent expiries to detect and surface (see R5).

## R3 — Catalog + download data shape

- **Decision**: Two endpoints drive the feature:
  - **Search**: `POST /api/v1/action/full_search/...` (body `query=`) and `POST
    /api/v1/action/advanced_search/...` (body `parameters=<json>` with `type`, `quality`, `genre`,
    `language`, `country`). Returns `result.page`/`result.pages` + `result.posts[]` with `id`, `title_type`
    (movie/series/anime), `is_series`, `title`, `imdb_id`, `imdb_score`, `30nama_score`, `image.cover`,
    `genre[]`, and capability flags (`free_download`, `coming_soon`, …).
  - **Download (movies)**: `POST /api/v1/action/download/id/{id}` → `result.download[]` where each entry has
    `quality` (e.g. "x265 BluRay REMUX 2160p"), `size`, `resolution`, `encoder`, `hardsub`, and a signed URL.
- **Rationale**: Verified: advanced_search returned 24 posts/page with those fields; download returned per-
  quality entries + signed links for a released movie.
- **Alternatives**: Parsing SSR `__NUXT__` HTML — rejected in favor of the cleaner JSON API.

## R4 — Signed download links & DSM fetch

- **Decision**: Generate the signed link at **send time** and hand it straight to Download Station via the
  existing `CreateTaskURIs`; do not cache links.
- **Rationale**: Verified the links embed **IP + expiry + HMAC**, sit on a **non-Cloudflare** storage host,
  support **HTTP range/resume** (`206`, `Accept-Ranges: bytes`), and download with **no cookies/token** — a
  `HEAD` returned `200`, ~11 GB, real filename. They are bound to the public IP that requested them; the NAS
  shares that IP, so DSM fetches them directly. Validity window ≈ weeks.
- **Implication**: Links are short-lived relative to nothing user-visible but MUST NOT be persisted/cached; a
  stale or wrong-IP link fails the download → reported, not silently retried.

## R5 — Session longevity & expiry handling

- **Decision**: Treat provider calls as fallible: map a bot-protection **challenge** or an **API 401/404-as-
  unauth** to a typed `ErrNeedsRefresh` (distinguishing clearance-layer vs. token-layer where possible) and a
  wrong-IP signature to `ErrIPMismatch`; surface both to clients as a single "source session needs refreshing"
  state, and to admins as an actionable refresh prompt. Persist a `state` + `last_verified_at` on the provider
  row.
- **Rationale**: Both layers expire; a background longevity monitor is measuring real TTLs to inform default
  re-verify cadence, but correctness does not depend on the number — the design detects failure and prompts a
  refresh regardless.
- **Alternatives**: Silent auto-retry (rejected — masks the real "re-paste needed" action); a keep-alive ping
  (may extend the token but not the clearance, so it's an optimization, not a correctness mechanism — deferred).

## R6 — Persistence & encryption

- **Decision**: One append-only migration (0005) adds `source_providers` (non-secret config/status),
  `source_provider_secrets` (`session_enc` sealed via the existing `store.Cipher`), and `source_prefs`
  (per-user preferred quality). Session material is **write-only** (never returned).
- **Rationale**: Reuses the exact NAS-password precedent (`Cipher.Seal`/`Open`, `SECRETS_KEY`). One volume,
  one store (Principle III).
- **Alternatives**: Env-var config (rejected — secrets must rotate at runtime via admin, and be encrypted at
  rest, not baked into deploy).

## R7 — Provider-neutral abstraction

- **Decision**: `internal/source` defines a `Provider` interface (`Search`, `TitleDownloads`, `VerifySession`)
  and a registry keyed by `kind`; the first site is a driver `providers/thirtynama.go`. Config carries base/
  API hosts, allowed download hosts, required session fields, and parent-folder mapping.
- **Rationale**: Keeps the public AGPL core free of one hardcoded site; isolates the outbound surface for
  testing and host-allowlisting.
- **Alternatives**: Hardcoding the site into handlers — rejected (neutrality + testability).

## R8 — Reuse of NAS folder/task flow & least privilege

- **Decision**: Send-to-NAS uses existing `CreateFolder` (per-title subfolder under the admin-designated
  movies parent) + `CreateTaskURIs` (task with `destination`), and validates the destination with
  `authz.AllowedForCreate(isAdmin, grants, dest)` exactly as task-create does.
- **Rationale**: No new download mechanism; per-user folder grants enforced server-side (Principle III least
  privilege).

## R9 — Outbound host allowlist

- **Decision**: The `source` client refuses any request whose host is not in the provider config's declared
  API/download host set. No client-supplied target hosts ever reach the client.
- **Rationale**: Preserves the "no open proxy" guarantee while permitting the one new, bounded outbound target.

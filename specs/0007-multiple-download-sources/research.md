# Research: Multiple Download Sources

**Feature**: 0007-multiple-download-sources | **Date**: 2026-09-02

Phase 0 output. Everything in the zarfilm sections was established by live calls against the
real site on 2026-09-02 with an operator-supplied logged-in session, not inferred from
documentation — the site publishes none.

---

## R1. Lifting the provider singleton

**Decision**: Make the provider a first-class row set. `store.GetProvider()` becomes
`ListProviders()` / `GetProviderByID(id)`; `SaveProviderConfig` stops upserting the lowest id
and takes an explicit id for update, insert for create. Every `/v1/source/*` route that acts on
a specific source gains a source identifier.

**Rationale**: The `source_providers` table already has an `id` primary key and the
`source_provider_secrets` table already keys off `provider_id` — the schema was built for many
and only the accessors assume one. So this is an accessor and routing change, not a migration
of shape. The one genuine schema need is a stable, user-visible ordering for the dropdown.

**Alternatives considered**:
- *A second singleton slot ("primary" and "secondary")*. Rejected: caps the feature at two,
  and every call site would branch on which slot it meant.
- *Keeping the singleton and multiplexing inside one driver*. Rejected: it would put
  cross-source merging inside a provider driver, exactly where the abstraction says it must
  not live, and would make two instances of the same kind impossible.

**Migration**: An existing installation has exactly one row, which already has an id. It
becomes the first list entry with no data change. FR-004 is satisfied by doing nothing — but
it must be *tested*, because the risk is a new NOT NULL column with no default.

---

## R2. Source-qualified title identifiers

**Decision**: Title identifiers become `<providerID>:<providerTitleID>` on the wire. The API
splits on the first colon, resolves the provider, and passes the remainder to the driver
untouched.

**Rationale**: `GET /v1/source/title/{id}` currently carries no source, and in combined mode
the client holds titles from several sources in one list — it must be able to hand one back
and have the server know where it came from. First-colon splitting keeps driver-side ids
opaque, which matters because zarfilm's ids are URL paths that themselves contain slashes
(`series/the-loyalty-game` versus `the-whisper-man-2026`) but never a colon.

**Alternatives considered**:
- *A separate `?source=` parameter*. Rejected: the id and its source must travel together or
  they can be recombined wrongly; one opaque string is harder to misuse.
- *Globally unique surrogate ids assigned by SynoDL*. Rejected: requires persisting a catalog,
  which the constitution forbids — the source stays the source of truth.

**Validation**: The provider portion must be checked against the caller's actually-configured
providers on every request, so a client cannot address a provider it should not see.

---

## R3. Combining results across sources

**Decision**: Merge server-side. For a combined query the API fetches page N from each enabled,
healthy source concurrently, then interleaves round-robin: first item of each source, second
item of each, and so on, skipping sources that have run out. Sources are queried with a
per-source timeout; one that fails or times out contributes nothing and is reported in a
`degraded` field alongside the results.

**Rationale**: Server-side is forced by custody — session material never leaves the server, so
the client cannot call sources itself. Concurrency matters because latency is otherwise
additive, and SC-005 caps combined at 50% slower than single-source; round-robin guarantees
SC-002 (both sources on the first screenful) regardless of how the sources' page sizes differ
(30nama and zarfilm return different counts per page).

**Alternatives considered**:
- *Sequential fetch*. Rejected: two sources would roughly double first-paint latency and blow
  SC-005.
- *Buffered global merge sort*. Rejected by the user during specification; costs over-fetching
  and a stateful cursor for an ordering guarantee that the single-source mode already provides
  exactly.
- *Client-side merge of per-source responses*. Rejected: multiplies round trips and pushes
  degradation logic into the view.

**Paging**: The combined response's page number is the per-source page number, shared. Because
sources have different page sizes, a combined page is simply the concatenation of what each
returned; the client keeps requesting N+1 until every source reports exhaustion. Titles cannot
repeat across pages because each source paginates independently and deterministically.

**Failure isolation**: A source erroring must not fail the request. The response carries the
items it could gather plus a list of degraded sources with a reason category, and the client
renders a non-blocking notice (FR-012).

---

## R4. Shared filter facets in combined mode

**Decision**: Each driver already reports its own facets through `Parameters`. For combined
mode the API intersects the facet *kinds* across enabled sources and, within a kind, keeps
options whose meaning matches. Matching is by the English slug where a driver supplies one and
by normalized label otherwise.

**Rationale**: FR-014 requires that every offered filter applies to everything on screen, and
`FacetOption` already carries both a provider-native `Name` and an English `Slug` precisely
because the existing source's labels are not English. Intersecting on slug is therefore
already supported by the data shape.

**Alternatives considered**:
- *A hardcoded canonical facet list*. Rejected: it would drift from what the sources actually
  offer, which is the reason `Parameters` exists at all.
- *Union with per-source application*. Rejected by the user during specification.

**Open risk**: Genre vocabularies may intersect to a small set. If the intersection turns out
to be uselessly thin in practice, the fallback is to widen matching with a small curated
synonym map in the API layer — noted, not built.

---

## R5. Session material shape

**Decision**: Generalize `source.Session` from its current header-shaped fields
(`CFClearance`, `CAPIKey`, `CToken`, `CPlatform`, `CAppVersion`) to a provider-neutral
key/value bag plus a `UserAgent`, with each driver declaring the fields it needs so the admin
UI can render the right form. `Client.Do` stops knowing provider-specific header names; the
driver supplies headers and cookies per request.

**Rationale**: zarfilm authenticates with a browser cookie, not headers. Today `Client.Do`
hardcodes `c-api-key`, `c-token`, `c-platform`, `c-app-version` and builds the `Cookie` header
solely from `cf_clearance` — none of which zarfilm can use, and all of which would be sent to
it as dead weight (and, worse, would leak the *other* source's material to a second site if a
session were ever mis-assigned). Making the driver own its auth is both the correct
factoring and a containment measure.

**Alternatives considered**:
- *Adding a `Cookie` field beside the existing ones*. Rejected: the struct grows a field per
  site forever, and every driver keeps seeing every other driver's secrets.
- *Per-driver session structs with type assertions*. Rejected: the store seals one JSON blob;
  a flat declared bag serializes cleanly and keeps the store provider-agnostic.

**Migration**: Existing sealed sessions are JSON with the current field names. The store must
read old blobs into the new bag by key so no operator has to re-paste (FR-004).

**Custody unchanged**: still sealed with the at-rest cipher, still write-only, still never
serialized to a client.

---

## R6. Reading zarfilm's catalog — HTML rather than JSON

**Decision**: Add `golang.org/x/net/html` and parse with its tokenizer. Per FR-029, correct
`CLAUDE.md:43` in the same change — not to amend a constraint, but because that line is already
wrong: it claims the server has "zero third-party dependencies, no database" while `go.mod`
requires `modernc.org/sqlite` directly and the entire store is SQLite. No constitution edit is
needed; its only relevant rule is that an added dependency be justified, which this section
does.

**Rationale**: zarfilm serves no structured data — `/wp-json/` is 404 and there is no separate
API host (`api.`, `app.`, `interface.`, `mobile.` all NXDOMAIN). Its catalog markup is regular
but deeply nested, and regex parsing of nested markup is the classic way to ship a subtly wrong
parser that fails on the one title with an unusual field. `x/net/html` is maintained by the Go
team, has no transitive dependencies beyond the standard library, and is the closest thing to a
standard-library answer available. It joins one existing direct dependency rather than breaking
a clean slate.

**Alternatives considered**:
- *Standard library only*. Considered seriously and rejected by the user; the markup is regular
  enough to make it work but the result is materially more fragile and less readable.
- *`goquery`*. Rejected: pulls in `cascadia` and a jQuery-style API far larger than needed.
- *Headless browser*. Rejected outright: enormous dependency, and unnecessary — every page
  needed renders server-side.

---

## R7. zarfilm — site facts

All verified live on 2026-09-02.

**Platform**: WordPress, theme `zarfilm208`, LiteSpeed cache, Cloudflare in front. No REST API.
Anonymous page loads did **not** hit a bot challenge, so unlike the existing source there is no
clearance cookie to carry — a meaningful simplification.

**Authentication**: cookie `wordpress_logged_in_<hash>` (a `_lscache_vary` cookie accompanies it
and selects the logged-in cache variant, so it must be sent too or a cached anonymous page can
come back). Login at `/sign-in/` is a form POST of `username`, `password`, `captcha`,
`security_login` (nonce) and `logintosite=true`, with the captcha image served from
`/captcha/?rnd=`. Human-only by design; SynoDL never touches it (FR-018). Observed cookie
lifetime ~2 weeks.

**Session verification**: every page inlines `var ajax_var = {...}` containing `"u"` (user id,
`"0"` when anonymous) and `"logged"` (`"1"` when logged in, empty otherwise). `VerifySession`
fetches one cheap page and reads those two fields. This distinguishes *not logged in* from
*logged in*; it does **not** distinguish *subscribed* from *unsubscribed* — see below.

**Subscription state**: an unsubscribed (or anonymous) session renders each download row as
`<a class="dllink vip_link" href="https://zarfilm.com/pricing/">`. A subscribed session renders
the real link. So the unsubscribed state (FR-019) is detected by fetching one title page and
finding `vip_link` where a real link was expected — a second, deliberate probe, not a guess.

**Catalog listing** (`/all-movie/page/N/`, and `/series/` for series; genre archives at
`/genre/<slug>/`): 21 items per page, ~1500 pages, verified zero overlap between page 1 and
page 2. One item is:

```html
<div class="inner_item_body_widget">
  <a class="bgbackitem" href="https://zarfilm.com/the-sheep-detectives-2026/" title="…">
    <div class="genres_links"><h3><span>خانوادگی</span></h3>…</div>
    <img src="https://zarfilm.com/wp-content/uploads/…-207x310.jpg" …>
  </a>
  <div class="item-foot-title">
    <h3 class="movie-title">The Sheep Detectives</h3>
    <div class="score">
      <span class="year">2026</span>
      <span class="rate">7.6<span class="ten">/10</span></span>
    </div>
  </div>
</div>
```

Note the title is in English while genres are Persian. The listing carries no IMDb id and no
plot; both come from the title page. Posters are served from `zarfilm.com` itself, so the image
proxy allowlist gains that host and no separate CDN.

**Sort**: exposed as `data-filter` values on the archive controls — `newest`, `modified`,
`imdb_rate`, plus numeric codes. There is also `wp-admin/admin-ajax.php?action=orderbyarchive`
returning `{posts, pages}` HTML fragments, but it **requires a nonce** (returned 403 without
one). Decision: use the plain paginated URLs, which need no nonce and no XHR headers. Simpler,
fewer moving parts, and one fewer thing to break.

**Text search**: `/?s=<term>` returns a full results page.
`wp-admin/admin-ajax.php?action=searchajaxstr` (POST `str=`) returns a JSON-wrapped HTML
fragment and worked without a nonce, but returns a typeahead subset. Decision: use `/?s=` for
real search and keep the ajax endpoint unused.

**Title page — movies** (`/<slug>/`): download rows are grouped by
`<div class="inner_dl_box_n_single">`, one group per release variant, headed by an `<h3>` naming
the variant in Persian ("نسخه زیرنویس فارسی چسبیده" = Persian-subtitled,
"نسخه دوبله فارسی" = Persian-dubbed) with a badge class of `subtitle_row` or `double_row`.
Inside, each `<div class="item_row_dl">` holds the `<a class="dllink …">` and a `.meta_row` of
label/value pairs: encoder (`Pahe`, `PSA`), size (`2.32 GB`), subtitle type (`SoftSub`).
Resolution is **not** in the metadata — it is in the link's filename (`1080p`, `720p`, `480p`),
so the driver parses it from there. IMDb id appears on the page (`tt11561116`) and in poster
filenames.

**Title page — series** (`/series/<slug>/`): the same structure nested one level deeper —
`row_season_n_dl` per season with a `season_name`, containing `item_part` entries per episode.
One sampled series carried 42 download links. This maps onto the existing `QualityOption`
`Season` / `Episodes` fields already used for the other source's season packs.

**Title identifiers**: movies live at `/<slug>/` and series at `/series/<slug>/`, so the
driver's title id is the URL path (`the-whisper-man-2026`, `series/the-loyalty-game`) — never
just the slug, or the two namespaces collide.

**Download links**:

```
https://dl6.indllserver.info/Movies9/2026/The.Whisper.Man.2026/
  The.Whisper.Man.2026.1080p.10bit.WEB-DL.6CH.x265.PSA.SoftSub.ZarFilm.mkv
  ?md5=<hash>&u=<userid>&expires=<unix>
```

Observed TTL ~18 hours, so links are resolved at send time (FR-022) — which is what
`ResolveDownload` already exists for. The host is `indllserver.info` under a numbered subdomain
(`dl6.`), so it is allowlisted by domain suffix exactly as the other source's rotating storage
domain is. **`zhomis.info` appears as a `dns-prefetch` hint on title pages and is not the
download host** — allowlisting it would widen the outbound surface for nothing.

**Link portability — verified**: a `HEAD` with no cookie and a `curl/8` User-Agent returned
`200`, `Content-Length: 686147352`, `Accept-Ranges: bytes`. So Download Station can fetch it
with no session material (FR-023), and range requests work, which Download Station relies on.

**Link address-binding — UNVERIFIED**: the check above ran from the same public address that
minted the link, so address-binding cannot be ruled out. This must be settled during
implementation by sending one link to a real NAS. If links prove address-bound, deployments
where SynoDL and the NAS egress differently would fail at download time, and the failure must
be reported distinctly rather than as a generic source error. Tracked as a risk, not a blocker.

---

## R8. Credential-free development and test path

**Decision**: Extend `internal/synomock` with a fake source site serving both providers' shapes
— a JSON API mimicking the existing source and an HTML site mimicking zarfilm — from the same
mock binary that already fakes DSM, on its existing port under a distinct path prefix. Its
fixtures are trimmed captures of the real responses. Drivers reach it because their host
allowlists are overridable in test builds only.

**Rationale**: `make start` and the e2e harness already boot `synomock` and must never need
real hardware (constitution: Mock-DSM dev parity). Sources are the second outbound target and
deserve the same treatment. Crucially, e2e boots `synodl` **stateless**, so a stored session
cannot exist there — the mock must be reachable in a way that does not depend on stored
provider config, and the e2e coverage of FR-026 must therefore include booting the e2e stack
with the source feature enabled against the mock.

**Alternatives considered**:
- *A separate mock binary per source*. Rejected: another process for `make start` to
  supervise, for no gain — the mock already multiplexes by path.
- *Recorded HTTP fixtures replayed in-process only*. Kept **as well**, for driver unit tests
  (the existing `_test.go`-per-file pattern), but insufficient alone: it cannot exercise the
  client, the merge, or the UI.

**Host allowlist safety**: the override must be impossible in a production build. It is
gated on a test-only mechanism, never an environment variable a deployed operator could set,
so the allowlist remains structural (constitution Principle III).

**Live path**: extend the existing `live_test.go` env-var pattern with a zarfilm equivalent —
skips unless its session variables are set, never runs in CI, and covers verify, browse,
search, title, and link resolution against the real site.

---

## Risks carried into implementation

| # | Risk | Mitigation |
|---|---|---|
| 1 | Signed links may be address-bound, breaking split SynoDL/NAS deployments | Send one to a real NAS early; report distinctly if confirmed |
| 2 | zarfilm markup can change without notice | Live checks catch drift; parse failures degrade to a source-level error, never a crash |
| 3 | Facet intersection may leave too few shared filters to be useful | Measure once both sources are live; curated synonym map as fallback |
| 4 | Session-shape migration could strand an existing operator's pasted material | Read old sealed blobs by key; cover with an explicit migration test |
| 5 | Combined latency could breach SC-005 | Fetch sources concurrently with a per-source timeout; degrade rather than wait |

# Phase 0 Research: Who made it — cast and director on a title

Everything below was read off the live sites on **2026-09-16**, not inferred from
the existing fixtures (which predate these blocks and are trimmed to the download
tables). Reproduced here because the drivers have to be written against the real
shapes, and because a future redesign wants a record of what was true.

---

## R1 — Does 30nama publish cast and director, and where?

**Decision.** Yes. Read them from `GET /api/v1/action/single/id/{id}` on
`interface.30nama.com` — the endpoint the site's own SPA uses, and one the driver
does not call today (`Title()` only calls `download/id/{id}`).

**Evidence.** The site is a Nuxt app; its SSR payload is the API response. Read
from the payload in a browser (a plain server-side `curl` gets 403 from
Cloudflare — the driver's stored session material is what gets past that, exactly
as it already does for search):

```jsonc
// result (abridged — 60+ fields)
{
  "id": …, "title": …, "year": …, "year_end": …, "title_type": …,
  "english_plot": …, "persian_plot": …, "imdb": "tt…", "imdb_score": …,
  "cast": [
    { "id": "416492",            // 30nama's own person id; NULL when they have no person record
      "name": "Alan Ritchson",
      "as":   "Jack Reacher",    // the character — no other source publishes this
      "imdb": "nm2024927",       // present even when "id" is null
      "order": 0,
      "image": { "cover": false, "poster": {
          "small"|"medium"|"large"|"big" (+ each "_webp"):
          "https://cdn.30nama.com/person/30416492-m_30NAMA.webp?" } } }
  ],
  "director": [ { "imdb": "nm0230032", "name": "Pete Docter", "order": 0 } ],
  "creator":  null,
  "writer":   [ { "imdb": "nm0428873", "name": "Mike Jones", "order": 1 } ]
}
```

**Three facts that shape the design:**

1. **`director` is `null` for series.** *Reacher* has no director and no creator;
   only `writer` is populated. *Soul* has two directors. This is why the spec
   covers creator and writers, not just director — a series would otherwise show
   a cast and nothing else.
2. **Crew entries never carry an image.** `director`, `creator` and `writer` are
   `{imdb, name, order}` and nothing more. Every crew face therefore comes from
   the IMDb fallback or not at all.
3. **The cast image is frequently a stand-in.** Its URL sits under `/none/`
   (`https://cdn.30nama.com/none/none-m_30NAMA.webp?2`). *Soul*: 6 of 6 people.
   *Reacher*: 3 of 6. Detecting that path is the whole of FR-013 for this source.

**Alternatives considered.** Scraping `30nama.com/movie/{id}/{slug}` for the
rendered cast section: rejected — it means parsing a Nuxt payload out of HTML, it
is a second host, and the API answer is both cleaner and already authenticated.

---

## R2 — Does ZarFilm publish them, and at what cost?

**Decision.** Yes, and for the **names** at zero cost: they are in the title page
`Title()` already fetches.

```html
<div class="single_casts">
  <div class="stars">
    <div class="label"><svg …><span>ستارگان: </span></div>      <!-- cast -->
    <div class="list">
      <div class="item"><a title="Kurt Russell" href="https://zarfilm.com/actor/kurt-russell/">Kurt Russell</a></div>
      …
    </div>
  </div>
  <div class="stars">
    <div class="label"><span>کارگردان: </span></div>            <!-- director -->
    <div class="list"><div class="item"><a href="https://zarfilm.com/director/andrew-patterson/">Andrew Patterson</a></div></div>
  </div>
  <div class="stars"><div class="label"><span>کشور: </span></div>…</div>  <!-- country — ignore -->
</div>
```

Observed: about three top-billed cast, names in Latin script, **no character
names**, **no photo**, **no IMDb id** on the title page. The `.stars` block is
also used for country (and other facts), so the groups must be told apart by
their Persian label and unrecognised labels ignored — FR-012.

**The person page is where identity lives.** `GET /actor/<slug>/` (and
`/director/<slug>/`) yields both halves in one request:

- `.linktoimdb a[href]` → `https://www.imdb.com/name/nm0000621/` — the IMDb id.
- `.inner_profile img` → `https://zarfilm.com/wp-content/uploads/2023/03/MV5B….jpg`
  — a portrait, re-hosted on `zarfilm.com`, which is **already** an allowlisted
  image host for this driver. (The file name is the IMDb still it was taken from,
  which is a neat confirmation that IMDb is the right fallback origin.)

**Placeholder tell:** the theme ships stand-in portraits at
`/wp-content/themes/zarfilm208/images/{woman,man}.jpg`. A portrait under
`/wp-content/themes/` is a stand-in; a real one is under `/wp-content/uploads/`.

**Decision on when to fetch person pages.** Inline, during the title-detail
request, bounded: at most 8 per title, at most 4 concurrently, under a 2.5s
deadline for the whole batch, and only for people not already cached. Rationale:
a title has 3–4 people, they are on the host we just talked to with the session we
already hold, and one round trip buys **both** the IMDb link and a photo — so the
tile is complete on first view. Anything not resolved inside the deadline simply
has no id and no photo, and the next title resolves it from cache.

**Alternatives considered.**

- *A client-driven endpoint keyed by source ref* (`?source=3&ref=actor/kurt-russell`),
  resolving lazily like the IMDb photo does. Rejected: it makes an
  **unauthenticated** endpoint spend the operator's source session on a
  client-supplied path. Server-side and inline keeps the source session entirely
  under the server's control, which is the Principle III instinct.
- *Names only, resolve in the background for next time.* Rejected: first view of
  any ZarFilm title would have no links and no faces, which is most views.

---

## R3 — How is a photograph obtained from IMDb?

**Decision.** `GET https://www.imdb.com/name/{nm}/`, read
`<meta property="og:image">`, and serve the resulting `m.media-amazon.com` image.

```html
<meta property="og:image"
      content="https://m.media-amazon.com/images/M/MV5BMTk3…._V1_FMjpg_UX1000_.jpg">
```

- There is **no `ld+json`** on a person page (there is on a title page), so
  `og:image` is the only structured answer.
- The page is ~1.4 MB, but `og:image` is in `<head>`. The read is therefore
  **capped at 256 KB and stopped at `</head>`** (FR-020) — the answer is always in
  the first few KB.
- The `_V1_` segment is Amazon's image-transform marker: replacing the trailing
  `_V1_…\.jpg` with `_V1_UX300_.jpg` yields a tile-sized rendition (FR-022).
- **No API exists.** IMDb has no free public API; TMDb was considered and
  rejected because it needs an operator-supplied API key, which is a new secret,
  new setup, and a new failure the operator has to understand. This feature must
  work with no configuration.
- **It can be refused.** Datacenter IPs are sometimes blocked, and a `curl` from
  this workstation to the sources already gets 403 from their WAF. So the whole
  path is best-effort: a failure caches a short-lived "none" and the tile shows
  initials (FR-023, SC-006).

---

## R4 — Where does the photo endpoint live, and how is it bounded?

**Decision.** A **third** image proxy, with its own host rule, mirroring
`handleYtdlThumb` (spec 1034) rather than extending `handleSourceImage`.

That precedent is explicit about why, and it applies verbatim here: the catalog
poster proxy's allowlist is assembled from the *download-source drivers plus the
operator's configured mirrors*. IMDb is neither. Sharing one list would let an
operator editing a source change what this feature may fetch, and vice versa.
Two concerns, two lists (FR-018).

Differences from the ytdl thumb proxy, both forced by the fallback being a
*lookup* rather than a known URL:

- The client sends a **person id, not a URL** — `GET /v1/source/person/{imdbId}/photo`,
  validated against `^nm\d{6,9}$` before anything outbound exists (FR-019). A URL
  parameter would have been an open-relay shape with a host check bolted on; an
  id cannot name a host at all.
- It is **rate-limited** with the existing per-IP limiter (the one on
  `POST /v1/session` and `POST /v1/fs/upload`), and the outbound lookup is capped
  at 4 concurrent instance-wide (FR-021). The ytdl proxy needs neither because it
  fetches a URL the caller already had; this one can *cause* a third-party lookup.

Unauthenticated for the same reason both existing proxies are: an `<img>` tag
cannot send the session header, and gating it would mean a token in a URL.

---

## R5 — Where does the cache live?

**Decision.** Two tables in the existing SQLite store, plus an in-memory LRU and
a single-flight guard in front.

| | key | value | TTL |
|---|---|---|---|
| `person_photos` | IMDb id | resolved image URL, or `''` for "none" | 30 days found / 3 days none |
| `source_people` | (source kind, source ref) | IMDb id + source-hosted photo URL | 30 days |

- **Why persisted** — argued in the spec's Credential-Safety Impact: re-deriving
  after a restart means re-scraping a third party for hundreds of people, which is
  the behaviour most likely to get an instance blocked. It records a *completed
  lookup*, not live state, so Principle III's "job state belongs to the
  orchestrator" rule does not apply — nothing about it can drift.
- **Why two tables** — the two lookups answer different questions and one is
  source-scoped. Folding a source ref into the IMDb-keyed table would mean rows
  with no IMDb id in a table whose primary key is one.
- **Bounding** (FR-029): expired rows are deleted on write, and a row cap
  (20 000 per table) drops the oldest beyond it. At ~100 bytes a row that is a
  couple of megabytes at worst.
- **Single-flight**: a hand-rolled in-flight map, not `golang.org/x/sync`. Adding
  a dependency for ~20 lines is exactly what CLAUDE.md says needs a spec-level
  justification, and it does not have one.

**Alternatives considered.** In-memory only (rejected: the restart case is the
one that matters, and the user asked for reuse explicitly). A blob cache of image
*bytes* in SQLite (rejected: that is what the byte LRU is for; bytes on the
operator's volume is real growth, a URL is not).

---

## R6 — How is any of this testable with no network?

**Decision.** Extend the in-repo mocks; add an IMDb mock beside them; gate the
redirect behind the existing `sourcemock` build tag.

- `synomock` gains `GET /mocksrc/tn/api/v1/action/single/id/{id}` returning the
  R1 shape — including a person with a `/none/` stand-in and a crew entry with no
  image, so the fallback path is exercised rather than argued about.
- The `zar` mock title page gains a `single_casts` block, plus
  `/mocksrc/zar/{actor,director}/{slug}/` person pages: one with a real portrait
  under `/wp-content/uploads/`, one with a theme stand-in, one with no IMDb link.
- `synomock` also serves a **mock IMDb**: `/mockimdb/name/{nm}/` with an
  `og:image` pointing at `/mockimg/{nm}.jpg`, and one id deliberately without one
  so "IMDb has no photo of this person" is a tested state.
- The IMDb base is redirected by `IMDB_MOCK_BASE`, read **only** in a file behind
  `//go:build sourcemock` — the same mechanism, and the same guarantee, as
  `mockBase()`: a release binary has no code path that could point this at
  anything but IMDb, whatever the environment says.

---

## R7 — What does the incidental gain cost?

**Decision.** Take `english_plot` / `persian_plot`, `year`, and `imdb` from the
same `single` response (FR-036).

30nama's `TitleDetail` returns an empty `Plot` and empty `IMDbID` today — the
sheet's synopsis and IMDb link for that source come only from the catalog row the
user happened to search through. The response this feature already fetches has
all three. Language rule (FR-037): prefer `english_plot`; fall back to
`persian_plot` only when English is empty — never both, never concatenated. The
client already renders synopses with `dir="auto"`, so a Persian fallback renders
correctly.

`TitleDetail` gains a `Year` field for this; the client prefers it over the year
it currently splits off the end of the title string.

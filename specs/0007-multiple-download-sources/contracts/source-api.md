# Contract: Multi-source catalog API

**Feature**: 0007-multiple-download-sources | **Date**: 2026-09-03

Amends the spec 0005 contract (`specs/0005-source-catalog/contracts/source-api.md`), which
assumed a single configured provider. All routes stay under `/v1/source`, remain
session-authenticated, and continue to return no secret material of any kind.

Backwards compatibility is deliberate: a client that knows nothing about multiple sources keeps
working against a single-source installation.

---

## Admin: managing sources

### `GET /v1/source/providers`

Lists configured sources. Admin only.

```json
{
  "providers": [
    { "id": 1, "kind": "thirtynama", "displayName": "30nama", "enabled": true,
      "state": "active", "lastVerifiedAt": 1788449147, "lastError": "",
      "sortOrder": 0, "moviesParent": "video/movies", "tvParent": "video/tv" },
    { "id": 2, "kind": "zarfilm", "displayName": "ZarFilm", "enabled": true,
      "state": "unsubscribed", "lastVerifiedAt": 1788449200, "lastError": "unsubscribed",
      "sortOrder": 1, "moviesParent": "video/movies", "tvParent": "video/tv" }
  ],
  "kinds": [
    { "kind": "thirtynama", "name": "30nama",
      "sessionFields": [ { "key": "cf_clearance", "label": "cf_clearance cookie",
                           "secret": true, "required": true, "help": "…" } ] },
    { "kind": "zarfilm", "name": "ZarFilm",
      "sessionFields": [ { "key": "wordpress_logged_in", "label": "Login cookie",
                           "secret": true, "required": true,
                           "help": "Grants full access to your site account…" } ] }
  ]
}
```

`sessionFields` drives the admin form, so adding a driver needs no client change. No response
on any route ever echoes a session value back — fields are write-only (Principle III).

### `POST /v1/source/providers`

Creates a source. Body: `kind`, `displayName`, optional `moviesParent`, `tvParent`,
`sortOrder`, and a `session` object of `{key: value}` matching that kind's declared fields.
Verifies before persisting. → `201` with the provider object, or `422` with
`{"error":"verify_failed","reason":"invalid_session|unsubscribed|unreachable"}`.

### `PUT /v1/source/providers/{id}` · `DELETE /v1/source/providers/{id}`

Update (config and/or session; omitted session fields keep their stored value) and remove.
Deleting destroys the sealed secret by cascade and drops the source from any user's selection.

### Retained from 0005

`PUT /v1/source/session`, `DELETE /v1/source/session`, `PUT /v1/source/policy` continue to work
and address **the lowest-id source**, so an existing single-source client is unaffected. They
are the compatibility surface; new work uses the `/providers` routes.

---

## Catalog: browsing and searching

### `POST /v1/source/search`

Body gains one optional field:

```json
{ "query": "", "page": 1, "sort": "newest", "order": "desc",
  "filters": { … },
  "source": ""          // "" or absent = all enabled sources; otherwise a provider id
}
```

Response gains `degraded`, and every item gains its source:

```json
{
  "page": 1,
  "pages": 1501,
  "items": [
    { "id": "2:the-whisper-man-2026", "sourceId": 2, "sourceName": "ZarFilm",
      "type": "movie", "title": "The Whisper Man", "posterUrl": "…",
      "imdbId": "tt11561116", "imdbScore": 6.3, "…": "…" }
  ],
  "degraded": [ { "sourceId": 1, "name": "30nama", "reason": "needs_refresh" } ]
}
```

Contract points:

- **`items[].id` is source-qualified** — `<providerID>:<providerTitleID>`, split on the first
  colon only. zarfilm ids are URL paths containing slashes (`2:series/the-loyalty-game`).
- **A failing source never fails the request.** It appears in `degraded` with a category and
  contributes no items (FR-012). Only when *every* source fails does the route return an error,
  and then once, not once per source.
- **`pages`** in combined mode is the maximum across contributing sources — the client keeps
  requesting until `items` comes back empty and `degraded` explains any shortfall.
- **Ordering** is round-robin by the sources' `sortOrder`, exact within a source and
  approximate across them (FR-009). With `source` set, ordering is exactly that source's.
- **`reason`** is one of `needs_refresh`, `unsubscribed`, `unreachable`, `timeout`. Never an
  upstream message or URL.

### `GET /v1/source/parameters?source=`

Without `source`: facet options **intersected** across enabled sources, matched by English slug
where a driver supplies one and by normalized label otherwise (FR-014). With `source`: that
source's own full facet set (FR-015).

### `GET /v1/source/title/{id}`

`{id}` is the source-qualified identifier. The provider portion is validated against configured
providers on every request, so a client cannot address a source it should not see. Response
gains `sourceId` / `sourceName`; `qualities` are unchanged in shape.

### `POST /v1/source/send`

Body's `titleId` is source-qualified; `qualityId` stays opaque to the driver. The link is
resolved at send time and never reused from when the title was viewed (FR-022). What reaches
the NAS is a resolved URL and a destination folder — no session material, ever (FR-023).

Errors keep their 0005 categories, plus `unsubscribed`.

---

## View state

### `GET /v1/source/view` · `PUT /v1/source/view`

Gains `selectedSource` (`""` = all sources) alongside the existing filters and sort, so the
selection follows the user across devices (FR-008). An unknown or removed source read back is
normalized to `""` rather than erroring.

`GET /v1/source/prefs`, `PUT /v1/source/prefs` and `GET /v1/source/quota` are **unchanged** —
preferred quality and the daily quota stay global across sources.

---

## Image proxy

`GET /v1/source/image` is unchanged in shape. Its allowlist is the union of every registered
driver's declared image hosts, which is safe because it is an unauthenticated proxy restricted
to known public poster CDNs. zarfilm serves posters from its own host, adding `zarfilm.com`.

Note the asymmetry, which is deliberate: **catalog** calls are checked against the *specific*
source's allowlist, never a union — a union there would let one source's compromise widen
another's outbound reach.

---

## Compatibility summary

| Client knows multi-source? | Behavior |
|---|---|
| No, one source configured | Identical to today: no `source` sent, one source answers, `degraded` empty and ignored |
| No, several configured | Combined results with qualified ids; ids stay opaque to a client that just echoes them back |
| Yes | Full behavior: selector, per-source narrowing, degraded notice |

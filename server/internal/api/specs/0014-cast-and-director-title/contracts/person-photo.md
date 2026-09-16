# Contract: person photo endpoint + title detail additions (spec 0014)

## `GET /v1/source/person/{imdbId}/photo`

Serves one person's photograph, same-origin, resolving it from IMDb on first ask.

**Auth**: none — an `<img>` tag cannot carry the session header, the same
constraint `/v1/source/image` and `/v1/ytdl/thumb` already live under. What
protects it is the id shape, the rate limit, and the fixed outbound allowlist.

**Rate limit**: the existing per-IP limiter (`LoginPerMinute`), the one already on
`POST /v1/session` and `POST /v1/fs/upload`.

| Parameter | Shape | On violation |
|---|---|---|
| `imdbId` (path) | `^nm\d{6,9}$`, lowercase | `400 bad_person_id`, before any outbound request exists |

**Responses**

| Status | When | Body / headers |
|---|---|---|
| `200` | A photograph was found (cached or freshly resolved) | image bytes; `Content-Type` from upstream (default `image/jpeg`); `Cache-Control: public, max-age=604800, immutable`; `X-Cache: HIT\|MISS` |
| `404` | IMDb has no photograph of this person, or the lookup failed/timed out/was refused | empty. **This is a normal outcome, not an error** — the client renders initials on it (FR-023, FR-031) |
| `400` | The id is not an IMDb person id | `bad_person_id` |
| `429` | Rate limited | standard limiter response |

**Outbound allowlist — this feature's own, deliberately not the poster proxy's:**

| Host | Purpose | Bound |
|---|---|---|
| `www.imdb.com` | `GET /name/{nm}/`, read `<meta property="og:image">` | read capped at 256 KB and stopped at `</head>`; 10s timeout |
| `m.media-amazon.com` | fetch the image named by `og:image` | 8 MB cap; 15s timeout |

Both `https` only. No client-supplied host, URL, or path reaches either. Outbound
lookups are capped at 4 concurrent instance-wide; concurrent asks for the same
unresolved person share one lookup.

A photograph URL is rewritten to a tile-sized rendition before fetching:
`…_V1_<anything>.jpg` → `…_V1_UX300_.jpg`, applied only to a URL that matched the
`m.media-amazon.com` `MV5B…` shape.

Under a `sourcemock` build ONLY, `IMDB_MOCK_BASE` redirects both hosts at the
in-repo mock. A release build contains no such code path.

---

## `GET /v1/source/title/{id}` — additions

Unchanged in shape; gains five optional fields. A title whose source publishes no
people serialises exactly as it does today.

```jsonc
{
  "id": "3:the-rivals-of-amziah-king-2025",
  "type": "movie",
  "title": "…",
  "sendable": true,
  "qualities": [ … ],          // unchanged
  "imdbId": "tt…",             // now also populated for the API-backed source
  "plot": "…",                 // now also populated for the API-backed source
  "year": "2025",              // NEW — FR-036

  "cast": [                    // NEW — billing order preserved
    { "name": "Alan Ritchson",
      "character": "Jack Reacher",   // omitted where the source publishes none
      "imdbId": "nm2024927",         // omitted where unknown → tile is not a link
      "photoUrl": "https://cdn.…/person/….webp" }  // omitted for a stand-in or none
  ],
  "directors": [ { "name": "Pete Docter", "imdbId": "nm0230032" } ],
  "creators":  [ … ],
  "writers":   [ … ]
}
```

Rules the handler and drivers must hold:

- A field is **omitted entirely** when the source publishes nothing for that role
  (FR-004a, FR-005) — never `[]`, never a placeholder entry.
- `photoUrl` is present only when the source's image is a real photograph.
  A provider stand-in (30nama's `/none/` path, ZarFilm's `/wp-content/themes/`
  portrait) MUST be dropped, not forwarded (FR-013).
- A source's internal handle for a person is never serialised (`Ref` is `json:"-"`).
- Failure to determine any of this MUST leave the rest of the response exactly as
  it is today (FR-006).
- Caps: 20 cast, 10 per crew role.

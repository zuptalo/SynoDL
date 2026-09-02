# Quickstart: Running two download sources locally

**Feature**: 0007-multiple-download-sources | **Date**: 2026-09-03

Two ways to run this feature. The first needs nothing at all; the second needs live session
material and is how you catch the real sites drifting.

---

## 1. Credential-free (the default)

```sh
make start
```

Brings up the mock DSM (`:8291`), `synodl` (`:8280`) and Vite (`:5273`) as before. The mock now
also serves **two fake source sites** on the same port under distinct path prefixes: one
speaking the JSON shape of the existing source, one speaking zarfilm's HTML shape. Their
fixtures are trimmed captures of real responses, so the drivers exercise their real parsing
paths.

1. Log in at http://localhost:5273 with `admin` / `secret`.
2. Settings → Sources → **Add source**. Add both mock sources; any session value is accepted by
   the mock, so paste anything non-empty.
3. Open Discover. The dropdown reads **All sources**; results from both are interleaved and
   each carries its source label.

This is the path CI runs, and it is enough to develop every part of the feature except the
real sites' quirks.

### Reproducing the interesting states

The mock's `/__mock/*` control endpoints drive the states that are otherwise hard to reach:

| To see | Do |
|---|---|
| A source needing refresh | Mark one mock source logged-out; Discover keeps showing the other, with a notice |
| The unsubscribed state | Mark the zarfilm-shaped mock as paywalled — download rows come back as upsell links |
| A slow / unreachable source | Ask the mock to stall past the per-source timeout; combined results still render |
| Exhaustion at different rates | Give one mock fewer pages; "load more" continues from the other |

---

## 2. Against the real sites

Needs an account on each, and for zarfilm an **active subscription** — without one every
download row is a "buy a subscription" link and there is nothing to send.

### Capturing session material

Neither site can be logged into by SynoDL: the existing source sits behind bot protection, and
zarfilm's login requires a human-solved captcha. Both are authorized by pasting material from a
browser where you are already logged in.

**zarfilm**: log in at https://zarfilm.com/sign-in/, open any movie page, DevTools → Network →
reload, right-click the document request → Copy → **Copy as cURL**. The `-b '…'` blob is the
cookie and the `-H 'user-agent: …'` line is the User-Agent. You need `wordpress_logged_in_*`
and `_lscache_vary` — the latter selects the logged-in cache variant, and without it a cached
anonymous page can come back and look like an expired session.

> That cookie is a **full account credential**, not a scoped token: anyone holding it can do
> anything your site account can. Signed download links embed your account id too. Treat both
> as secrets, and log out at the site to invalidate a cookie you have finished with.

Paste into Settings → Sources → Add source → ZarFilm.

### Live driver checks

The driver-level checks run against the real sites when given credentials and **skip** — never
fail — without them, so they stay out of CI:

```sh
cd server
LIVE_ZAR_COOKIE='wordpress_logged_in_…=…; _lscache_vary=…' \
LIVE_ZAR_UA='Mozilla/5.0 …' \
  go test ./internal/source/providers/ -run TestLiveZarfilm -v
```

The existing source's live check is unchanged (`LIVE_CF=… go test -run TestLiveThirtynama`).

---

## Verifying the risky bit

One thing the mock cannot tell you: whether zarfilm's signed links are bound to the address
that requested them. They are confirmed fetchable with no cookie and a mismatched User-Agent,
but that check ran from the same public address that minted the link.

To settle it, send a real zarfilm download to a real NAS on a different egress path and watch
whether Download Station gets past 0%. If it stalls with an auth error, links are
address-bound, and deployments where SynoDL and the NAS leave through different addresses
cannot use this source.

---

## Gates before calling it done

```sh
npm run build                 # typecheck + build
npm run test:unit:coverage
cd server && go build ./... && go vet ./... && go test ./...
npm run test:e2e
```

`go test` must stay green with **no** `LIVE_*` variables set — if a live check ever fails in
CI, it has lost its skip guard.

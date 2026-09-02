# Quickstart: Running two download sources locally

**Feature**: 0007-multiple-download-sources | **Date**: 2026-09-03

Two ways to run this feature. The first needs nothing at all; the second needs live session
material and is how you catch the real sites drifting.

---

## 1. Credential-free (the default)

```sh
make start
```

Brings up the mock DSM (`:8291`, now over TLS), `synodl` (`:8280`) and Vite (`:5273`).

Two things changed to make this work at all, because the download sources exist only in the
server's stateful mode and that mode was previously unreachable locally:

- the dev backend boots **stateful**, with throwaway `SECRETS_KEY` and `DATA_DIR` (`.devdata/`,
  gitignored) — override either in `server/.env`;
- the mock DSM serves **TLS**, since stateful SynoDL always dials `https://` — the same shape as
  a self-signed NAS.

The mock also serves **two fake source sites** on the same port: `/mocksrc/zar` (HTML, like a
WordPress film site) and `/mocksrc/tn` (the other provider's JSON envelopes). They emit the same
class names, nesting and absolute links the real sites do, so the drivers run their real parsing
paths.

The dev build carries the `sourcemock` build tag, which is what lets a driver be pointed at a
fake site. A release build never passes that tag, so the capability does not exist in
production.

1. Open http://localhost:5273 and complete the first-run wizard (the NAS fields are pre-filled
   for the mock; the mock accepts `admin` / `secret`).
2. Settings → Download sources → **Add a source**. Choose ZarFilm and paste anything non-empty —
   the fake site accepts any value.
3. Open Discover. Add a second source the same way to see the selector appear, results
   interleaved, and each card labelled with its source.

Note the mock DSM's shares are `movie` and `tv-show`, so use those as the destination folders.

### Reproducing the interesting states

The mock's `/__mock/*` control endpoints drive the states that are otherwise hard to reach:

| To see | Do |
|---|---|
| A source needing refresh | `POST https://localhost:8291/__mock/source/zar/logged-out` — Discover keeps showing the other source, with a notice naming this one |
| The unsubscribed state | `POST …/__mock/source/zar/paywalled` — download rows come back as upsell links |
| Exhaustion at different rates | `POST …/__mock/source/zar/pages?n=2` — "load more" continues from the other source |
| Everything back to normal | `POST …/__mock/source/reset` |

(The mock speaks TLS, so pass `-k` to curl.)

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

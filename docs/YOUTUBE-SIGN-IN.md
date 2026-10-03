# Giving SynoDL a YouTube sign-in

YouTube sometimes refuses SynoDL's download workers with **"Sign in to confirm you're
not a bot"**. It treats an anonymous, automated client as suspicious and a signed-in
browser session as trustworthy. Saving a sign-in in SynoDL lets the workers present one.

## Getting the header

1. In a **private browser window**, sign in to YouTube and stay on `youtube.com`.
2. Open developer tools (F12) → **Network** → reload → click any request to
   `www.youtube.com` → **Request Headers** → copy the whole value of `Cookie`.
3. Close the private window right away, so the session is not replaced under SynoDL.

## Saving it

**Settings → YouTube sign-in** (admins only) → paste → **Save**. SynoDL says how many
cookies it found and whether the login cookies (`SAPISID`, `__Secure-3PSID`,
`LOGIN_INFO`, `SID`) are there; if none are, the paste probably came from a request that
was not signed in. A cookies file (Netscape format) works too.

It is **a live login for a Google account**: use an account you are comfortable with,
and remember that pulling many downloads through one session can get it flagged. It is
stored encrypted, never shown again, and removed with **Remove**.

## When it stops working

Google can end a session (or you sign out of that browser). If YouTube still refuses a
download with a sign-in saved, the download says *the saved YouTube sign-in appears to
have stopped working*, and Settings shows when it was last refused. Paste a fresh one.

A valid saved sign-in may still be refused when YouTube requires proof-of-origin
tokens for the cluster's egress address. Operators with a compatible worker image
can set `YTDL_POT_PROVIDER_URL` to an internal bgutil provider; see the Kubernetes
deployment guide. This supplements the saved cookies rather than replacing them.

## How it reaches a worker (for operators)

The cookies never enter a Job, so anyone who can read Jobs cannot read them. A Job carries
a single-use, ten-minute **grant**; an init container redeems it at
`/v1/internal/ytdl-signin` — honoured only for a request from that Job's own pod address,
never for one carrying a forwarding header, so nothing through the ingress can redeem
it — and writes the cookies to a memory-backed volume that goes with the pod. If the
fetch fails the download simply runs anonymously. No RBAC change: the server still has no
access to Secrets. Override the fetch URL with `SIGNIN_FETCH_URL` if the Service is not
named `synodl`.

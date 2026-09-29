# Quickstart: YouTube sign-in

1. In a private browser window sign in to YouTube; on `youtube.com` open DevTools → Network → any `www.youtube.com` request → Request Headers → copy the whole `Cookie` value.
2. SynoDL → Settings → **YouTube sign-in** → paste → **Save**. Expect "N cookies · login cookies: SAPISID, …".
3. Start a YouTube download. Its worker Job carries the label `synodl.io/signin=true`; `kubectl -n synodl get job <name> -o yaml` shows a grant but no cookie value.
4. If YouTube still refuses ("not a bot"), the download says the saved sign-in stopped working; paste a fresh one.
5. **Remove** deletes the stored copy.
Dev: `make start` runs the mock cluster; the modal and the Job shape can be exercised with no YouTube at all.

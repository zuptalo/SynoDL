# Quickstart: developing & verifying spec 0002

## Run locally

```sh
make start   # mock DSM (:8291) + synodl (air, :8280) + Vite (:5273)
```

Open http://localhost:5273, log in with `admin` / `secret`. The Tasks tab connects to the SSE stream;
watch the browser Network panel for a long-lived `GET /v1/tasks/stream` (type `eventsource`/`fetch`)
carrying the `X-Syno-Sid` header.

## See it live

```sh
# seed a moving download, then advance the mock clock and watch the row update with no refresh
curl -s -XPOST localhost:8291/__mock/seed -d '{"tasks":[{"name":"live.iso","type":"http","status":"downloading","size":1073741824,"rate":10485760}]}'
curl -s -XPOST localhost:8291/__mock/tick -d '{"seconds":5}'
```

## See a failure reason

```sh
curl -s -XPOST localhost:8291/__mock/seed -d '{"tasks":[{"name":"broken.iso","type":"bt","status":"error","errorDetail":"broken_link"}]}'
# the row shows "Broken link"; tap it → detail modal shows the same reason + full fields
```

## Gates (all must pass — Principle V)

```sh
npm run build                 # vue-tsc typecheck + vite build
npm run test:unit:coverage    # vitest + coverage floors (task-error.ts joins the allowlist)
cd server && go build ./... && go vet ./... && go test ./...   # syno floor 75, config floor 85
npm run test:e2e              # Playwright: live update, error reason, detail view, fallback
```

## Manual live-verify against the real NAS (post-merge, sid-safe)

After Keel rolls `:latest` to the k3s deployment, open the deployed app URL and confirm the
list updates without pull-to-refresh and the pre-existing errored task now shows a reason. Confirm the
exact DSM `error_detail` keyword set **without placing a sid in any log/transcript** (operator-run).

# Data Model: YouTube sign-in

## `youtube_signin` (migration 0041) — one row, `id = 1`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK CHECK (id = 1) | at most one sign-in |
| cookies_sealed | BLOB NOT NULL | `Cipher.Seal(netscape bytes)` |
| cookie_count | INTEGER NOT NULL | metadata |
| login_found | TEXT NOT NULL | comma-separated names |
| saved_at | INTEGER NOT NULL | unix seconds |
| saved_by_name | TEXT NOT NULL DEFAULT '' | snapshot, survives user removal |
| last_refused_at | INTEGER | last download refused WITH the sign-in |
| last_ok_at | INTEGER | last download that succeeded with it |

`CREATE TABLE IF NOT EXISTS` (the drift repair replays migrations). The golden migration hash is updated.

## In memory (not persisted)
`grant`: `sha256(token) → {job string, expires time.Time, used bool}`. Lost on restart; a download whose grant is lost simply runs anonymously.

## Job additions (no secret)
Label `synodl.io/signin=true`; init container `signin` (env `SYNODL_SIGNIN_URL`, `SYNODL_SIGNIN_GRANT`); volume `signin` (memory emptyDir, 1 Mi) mounted at `/signin` in both containers.

## State
`absent → saved → (refused ↔ ok) → absent`. Refused/ok are derived from finished Jobs and only recorded as timestamps.

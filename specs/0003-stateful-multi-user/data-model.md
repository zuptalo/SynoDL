# Data Model: Stateful multi-user rework (SQLite)

One SQLite database on the mounted volume (`DATA_DIR/synodl.db`). Secret columns are encrypted at rest
(AES-GCM under a key derived from `SECRETS_KEY`); passwords are salted hashes. Schema is created/upgraded
by an in-code migration runner (a `schema_migrations` table tracks the applied version).

## Tables

### `operator_config` (exactly one row; `id` fixed = 1)
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK CHECK(id=1) | singleton |
| public_url | TEXT | canonical app URL |
| nas_address | TEXT | host/IP |
| nas_port | INTEGER | |
| nas_tls_verify | INTEGER (bool) | 0 = allow self-signed (SYNO_TLS_INSECURE-equivalent) |
| nas_account | TEXT | NAS username |
| nas_password_enc | BLOB | **encrypted** NAS password |
| nas_uses_2fa | INTEGER (bool) | whether the account needs OTP |
| created_at / updated_at | INTEGER | unix seconds |

### `users`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| username | TEXT UNIQUE (case-insensitive) | |
| password_hash | TEXT | salted hash (scrypt/argon2 params encoded in the string) |
| is_admin | INTEGER (bool) | |
| is_enabled | INTEGER (bool) | disable = immediate lockout |
| created_at / updated_at | INTEGER | |

### `sessions`
| Column | Type | Notes |
|---|---|---|
| token_hash | TEXT PK | store only a hash of the opaque session token |
| user_id | INTEGER FK→users | ON DELETE CASCADE (delete/disable ⇒ sessions gone) |
| created_at / expires_at | INTEGER | sliding or fixed TTL |

### `folder_grants`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| user_id | INTEGER FK→users | ON DELETE CASCADE |
| folder_path | TEXT | allowed NAS path (a task may target this or a descendant) |
| UNIQUE(user_id, folder_path) | | |
Policy: admins default to all folders; a user with **no** grants can download nowhere until granted
(safe default). Enforcement: the picker lists only granted subtrees; task-create validates the
normalized destination is within a grant (reject traversal / out-of-scope) before any NAS call.

### `push_subscriptions`
| Column | Type | Notes |
|---|---|---|
| id | INTEGER PK | |
| user_id | INTEGER FK→users | ON DELETE CASCADE |
| endpoint | TEXT UNIQUE | push service URL |
| p256dh | TEXT | client public key (base64url) |
| auth | TEXT | client auth secret (base64url) |
| opted_in | INTEGER (bool) | per-device opt-in |
| created_at | INTEGER | |
Invalid endpoints (410/404 from the push service) are pruned.

### `instance` (singleton key/value or fixed row; instance-wide secrets/state)
| Column | Type | Notes |
|---|---|---|
| vapid_public | TEXT | base64url P-256 public key (served to clients) |
| vapid_private_enc | BLOB | **encrypted** P-256 private key |
| vapid_subject | TEXT | `mailto:` or the public URL |
| last_version_notified | TEXT | for app-update pushes (per-instance high-water mark) |

### `watched_tasks` (completion watcher bookkeeping)
| Column | Type | Notes |
|---|---|---|
| nas_task_id | TEXT PK | DSM task id |
| owner_user_id | INTEGER FK→users NULL | who created it via SynoDL (for push attribution); NULL if pre-existing |
| last_status | TEXT | last seen status; transition →finished triggers a push |
| notified | INTEGER (bool) | ensures exactly-once completion push |
| first_seen / updated_at | INTEGER | |

## Encryption at rest

- `SECRETS_KEY` (env) → 32-byte key via HKDF-SHA256. `*_enc` columns = AES-256-GCM (random nonce
  prepended). On boot, if a decrypt of a known canary/`operator_config` fails, SynoDL refuses to start
  with a clear "wrong or missing SECRETS_KEY" message (never silently resets).
- `SECRETS_KEY` is never written to the DB or logs.

## Client-side (unchanged mechanism)

- IndexedDB still holds the client's **own** SynoDL session token + UI prefs. Task state is not
  persisted. If a new object store is needed, bump `DB_VERSION` with a forward migration (Principle IV).

## Attribution & push

A completion push goes to the opted-in devices of the task's `owner_user_id` (the SynoDL user who
created it). Pre-existing/unattributed tasks (NULL owner) notify **admins** who opted in (configurable
default), never unrelated users. Exactly-once via `watched_tasks.notified`.

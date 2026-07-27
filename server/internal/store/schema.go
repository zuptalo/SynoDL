package store

// migrations are applied in order; the applied count is tracked in
// schema_migrations so an existing volume upgrades forward without data loss.
// Never edit a shipped migration — append a new one.
var migrations = []string{
	// 0001 — initial schema for the stateful multi-user rework (spec 0003).
	`
	CREATE TABLE operator_config (
		id              INTEGER PRIMARY KEY CHECK (id = 1),
		public_url      TEXT NOT NULL DEFAULT '',
		nas_address     TEXT NOT NULL DEFAULT '',
		nas_port        INTEGER NOT NULL DEFAULT 5001,
		nas_tls_verify  INTEGER NOT NULL DEFAULT 1,
		nas_account     TEXT NOT NULL DEFAULT '',
		nas_password_enc BLOB,
		nas_uses_2fa    INTEGER NOT NULL DEFAULT 0,
		created_at      INTEGER NOT NULL DEFAULT 0,
		updated_at      INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin      INTEGER NOT NULL DEFAULT 0,
		is_enabled    INTEGER NOT NULL DEFAULT 1,
		created_at    INTEGER NOT NULL DEFAULT 0,
		updated_at    INTEGER NOT NULL DEFAULT 0
	);
	CREATE UNIQUE INDEX idx_users_username ON users (username COLLATE NOCASE);

	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at INTEGER NOT NULL DEFAULT 0,
		expires_at INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX idx_sessions_user ON sessions (user_id);

	CREATE TABLE folder_grants (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		folder_path TEXT NOT NULL,
		UNIQUE (user_id, folder_path)
	);

	CREATE TABLE push_subscriptions (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		endpoint   TEXT NOT NULL UNIQUE,
		p256dh     TEXT NOT NULL,
		auth       TEXT NOT NULL,
		opted_in   INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE instance (
		id                    INTEGER PRIMARY KEY CHECK (id = 1),
		vapid_public          TEXT NOT NULL DEFAULT '',
		vapid_private_enc     BLOB,
		vapid_subject         TEXT NOT NULL DEFAULT '',
		last_version_notified TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE watched_tasks (
		nas_task_id   TEXT PRIMARY KEY,
		owner_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
		last_status   TEXT NOT NULL DEFAULT '',
		notified      INTEGER NOT NULL DEFAULT 0,
		first_seen    INTEGER NOT NULL DEFAULT 0,
		updated_at    INTEGER NOT NULL DEFAULT 0
	);
	`,
	// 0002 — cache the live NAS session id (encrypted) so a pod restart / deploy
	// doesn't drop it and force a 2FA re-auth every time (spec 0003 fix).
	`ALTER TABLE operator_config ADD COLUMN nas_sid_enc BLOB;`,
	// 0003 — per-user notification preferences + task-ownership claims (spec 1004).
	`
	CREATE TABLE notification_prefs (
		user_id          INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		notify_added     INTEGER NOT NULL DEFAULT 0,
		notify_completed INTEGER NOT NULL DEFAULT 1,
		notify_failed    INTEGER NOT NULL DEFAULT 1,
		scope            TEXT    NOT NULL DEFAULT 'own',
		updated_at       INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE task_claims (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name_hint  TEXT    NOT NULL,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX idx_task_claims_name ON task_claims(name_hint);
	`,
}

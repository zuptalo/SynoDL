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
	// 0004 — per-user destination preferences: a default folder + favorites,
	// moved server-side so they persist and sync across a user's sessions (spec
	// 1011). favorites is a newline-joined list of destination paths.
	`
	CREATE TABLE destination_prefs (
		user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		default_dest TEXT    NOT NULL DEFAULT '',
		favorites    TEXT    NOT NULL DEFAULT '',
		updated_at   INTEGER NOT NULL DEFAULT 0
	);
	`,
	// 0005 — download-source catalog (spec 0005): an admin-configured external
	// "source provider" whose session material is encrypted at rest (write-only,
	// like the NAS password) and whose non-secret config/status is plaintext.
	// source_prefs holds a per-user preferred quality. Off by default (enabled=0);
	// api_hosts/download_hosts are newline-joined outbound allowlists.
	`
	CREATE TABLE source_providers (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		kind             TEXT    NOT NULL,
		display_name     TEXT    NOT NULL DEFAULT '',
		api_hosts        TEXT    NOT NULL DEFAULT '',
		download_hosts   TEXT    NOT NULL DEFAULT '',
		movies_parent    TEXT    NOT NULL DEFAULT '',
		tv_parent        TEXT    NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 0,
		state            TEXT    NOT NULL DEFAULT 'not_configured',
		last_verified_at INTEGER NOT NULL DEFAULT 0,
		created_at       INTEGER NOT NULL DEFAULT 0,
		updated_at       INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE source_provider_secrets (
		provider_id INTEGER PRIMARY KEY REFERENCES source_providers(id) ON DELETE CASCADE,
		session_enc BLOB    NOT NULL,
		updated_at  INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE source_prefs (
		user_id           INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		preferred_quality TEXT    NOT NULL DEFAULT '',
		updated_at        INTEGER NOT NULL DEFAULT 0
	);
	`,
	// 0006 — per-user content-rating cap for the download catalog (spec 0005
	// parental controls). Empty = unrestricted; otherwise the server forces this
	// rating on the user's source searches so a scoped account only sees titles at
	// that rating.
	`ALTER TABLE users ADD COLUMN content_rating TEXT NOT NULL DEFAULT '';`,
	// 0007 — download quotas (spec 0005): an instance-wide maximum download size
	// (MB, 0 = unlimited) and a per-user rolling 24h download-count limit
	// (0 = unlimited), to balance quality vs. the provider's daily download cap.
	`ALTER TABLE operator_config ADD COLUMN max_download_mb INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE users ADD COLUMN daily_download_limit INTEGER NOT NULL DEFAULT 0;`,
	// 0008 — remember each user's Discover view (facet filters as JSON + the sort
	// order) on the server so it follows them across devices.
	`ALTER TABLE source_prefs ADD COLUMN filters TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE source_prefs ADD COLUMN sort TEXT NOT NULL DEFAULT '';`,
	// 0009 — remember the sort DIRECTION too ("asc"/"desc"; "" = the app default,
	// descending), so a reversed Discover sort follows the user across devices.
	`ALTER TABLE source_prefs ADD COLUMN sort_order TEXT NOT NULL DEFAULT '';`,
	// 0010 — a durable per-download log for the daily quota (spec 1013). The count
	// used to be derived from task_claims, but the watcher deletes those on
	// attribution, so the count decayed and couldn't back an accurate "remaining"
	// or an admin reset. One row per file/episode sent; an admin reset just clears
	// a user's rows.
	`
	CREATE TABLE download_events (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX idx_download_events_user ON download_events(user_id, created_at);
	`,
	// 0011 — remember the catalog metadata for a Discover send (spec 1013), keyed
	// by the destination subfolder, so the Tasks list can show whether a download
	// is a movie or series plus its IMDb rating and year. One row per title folder;
	// a re-send upserts.
	`
	CREATE TABLE source_downloads (
		destination TEXT PRIMARY KEY,
		media_type  TEXT NOT NULL DEFAULT '',
		title       TEXT NOT NULL DEFAULT '',
		year        TEXT NOT NULL DEFAULT '',
		imdb_score  REAL NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL DEFAULT 0
	);
	`,
	// 0012 — remember WHO sent each Discover download, keyed by its destination
	// folder. Download Station names a task after the file, not the folder, so the
	// name-based ownership claim never matched a source send — attribute those by
	// destination instead (reliable, like the metadata above).
	`ALTER TABLE source_downloads ADD COLUMN owner_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL;`,
	// 0013 — durable per-download history for the Statistics section (spec 0006).
	// One append-only row per downloaded file, written at create time (so paused/
	// canceled downloads still count, matching the daily-limit accounting) and
	// backfilled with the real size by the completion watcher. This is a statistics/
	// attribution log, NOT a live task mirror — the NAS stays the source of truth
	// for current task state; only a completion timestamp + final size are kept.
	// History begins at rollout: nothing seeds it from the size-less download_events
	// or the folder-deduplicated source_downloads. Rows cascade away with the user.
	`
	CREATE TABLE download_history (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		source       TEXT    NOT NULL,               -- 'catalog' | 'direct'
		category     TEXT    NOT NULL,               -- movie|series|anime|music_video|music|other
		destination  TEXT    NOT NULL DEFAULT '',    -- folder; correlation key for size backfill
		task_name    TEXT    NOT NULL DEFAULT '',    -- expected file name; correlation key
		created_at   INTEGER NOT NULL,               -- when the download was added
		completed_at INTEGER,                         -- set when the watcher observes it finish
		size_bytes   INTEGER,                         -- real size; NULL until completed
		task_id      TEXT                             -- DSM task id once correlated (diagnostic)
	);
	CREATE INDEX idx_download_history_user      ON download_history(user_id, created_at);
	CREATE INDEX idx_download_history_correlate ON download_history(destination, task_name, completed_at);
	`,
}

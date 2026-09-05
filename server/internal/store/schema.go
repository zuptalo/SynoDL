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
	// 0014 — remember the title's poster image for a Discover send (spec 1016), so
	// the Tasks list can show a poster thumbnail. Public catalog CDN URL, the same
	// image Discover already renders; empty for downloads sent before this shipped.
	`ALTER TABLE source_downloads ADD COLUMN poster_url TEXT NOT NULL DEFAULT '';`,
	// 0015 — remember the catalog title id for a Discover send (spec 1016), so the
	// task detail's "Open in Discover" can reopen the exact title. Empty for older
	// or manually-added downloads (they fall back to a title search).
	`ALTER TABLE source_downloads ADD COLUMN catalog_id TEXT NOT NULL DEFAULT '';`,
	// 0016 — multiple download sources (spec 0007). source_providers was always
	// keyed by id; only the accessors assumed a single row. These columns add what
	// many rows actually need:
	//   sort_order — a stable, operator-controlled order for the source selector
	//                AND for round-robin interleaving, so a combined list doesn't
	//                reshuffle between requests. Ties break by id, so the ordering
	//                is total even when an operator never sets it.
	//   last_error — the last failure CATEGORY for admin display
	//                (needs_refresh / unsubscribed / unreachable). A category only:
	//                never an upstream body, URL, or anything derived from a secret.
	// An existing single-provider install keeps its id and lands at sort_order 0,
	// i.e. first in the list, with no data change and nothing to re-paste.
	`ALTER TABLE source_providers ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE source_providers ADD COLUMN last_error TEXT    NOT NULL DEFAULT '';`,
	// 0017 — remember which source a user is browsing in Discover (spec 0007).
	// Empty = "All sources", so every pre-existing row already means the right
	// thing and so does the default for a new user. Stored as TEXT rather than an
	// FK on purpose: when a source is deleted the selection must degrade to "all"
	// on read, not break a constraint or need a migration to clean up.
	`ALTER TABLE source_prefs ADD COLUMN selected_source TEXT NOT NULL DEFAULT '';`,
	// 0018 — an operator-set alternate address for a source (spec 1020). These
	// sites get blocked periodically and publish a mirror; without somewhere to
	// put it, a routine outage silently removes a source the operator pays for.
	// Empty = no alternate, which is exactly today's behaviour.
	//
	// Note this is the ONE outbound host a source can reach that is not
	// provider-declared. It is administrator configuration, never client input,
	// and applies only to the source it is set on.
	`ALTER TABLE source_providers ADD COLUMN alt_base TEXT NOT NULL DEFAULT '';`,

	// Hide titles the user already has, or is already fetching, from Discover
	// (spec 0008 FR-022/FR-024). On the per-user prefs row so it follows them
	// across devices, like the rest of their Discover view.
	`ALTER TABLE source_prefs ADD COLUMN hide_owned INTEGER NOT NULL DEFAULT 0;`,

	// Spec 0009: the address a source is reached at becomes configuration rather
	// than something fixed in the driver. Empty keeps the driver's own address, so
	// every source configured before this keeps working untouched.
	`ALTER TABLE source_providers ADD COLUMN main_base TEXT NOT NULL DEFAULT '';`,
	// 0019 — the 30nama driver's registry key was "thirtynama", which leaked into
	// the admin list as the source's name. The key is now "30nama", matching what
	// the site is actually called.
	//
	// Existing rows MUST be rewritten here: kind is how a stored source finds its
	// driver, so a row left on the old key would find none and the operator's
	// configured source would go dark — with its sealed session intact but
	// unusable. The UPDATE is a no-op on an install that never had one.
	`UPDATE source_providers SET kind = '30nama' WHERE kind = 'thirtynama';`,
}

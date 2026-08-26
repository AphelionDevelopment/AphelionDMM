CREATE TABLE IF NOT EXISTS client_sessions (
	session_id TEXT PRIMARY KEY,
	relay_url TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('viewer', 'editor', 'owner')),
	document_id TEXT NOT NULL UNIQUE,
	owner_public_key BLOB NOT NULL CHECK (length(owner_public_key) = 32),
	manifest BLOB NOT NULL,
	manifest_sha256 BLOB NOT NULL CHECK (length(manifest_sha256) = 32),
	acknowledged_revision INTEGER NOT NULL,
	acknowledged_map_hash TEXT NOT NULL,
	display_name TEXT NOT NULL,
	identity_secret_ref TEXT NOT NULL,
	group_key_secret_ref TEXT NOT NULL,
	owner_secret_ref TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pending_submissions (
	session_id TEXT NOT NULL,
	operation_id TEXT NOT NULL,
	operation BLOB NOT NULL,
	disposition TEXT NOT NULL CHECK (disposition IN ('ready', 'conflicting', 'obsolete', 'submitted')),
	created_at TEXT NOT NULL,
	diagnostic TEXT NOT NULL,
	PRIMARY KEY (session_id, operation_id),
	FOREIGN KEY (session_id) REFERENCES client_sessions(session_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS actor_profiles (
	session_id TEXT NOT NULL,
	actor_key BLOB NOT NULL CHECK (length(actor_key) = 32),
	display_name TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	PRIMARY KEY (session_id, actor_key),
	FOREIGN KEY (session_id) REFERENCES client_sessions(session_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS admissions (
	session_id TEXT NOT NULL,
	capability_id BLOB NOT NULL CHECK (length(capability_id) = 32),
	role TEXT NOT NULL CHECK (role IN ('viewer', 'editor')),
	expires_at TEXT NOT NULL,
	bound_actor BLOB NOT NULL CHECK (length(bound_actor) = 32),
	PRIMARY KEY (session_id, capability_id),
	FOREIGN KEY (session_id) REFERENCES client_sessions(session_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS pending_submissions_by_state
	ON pending_submissions(session_id, disposition, created_at);

PRAGMA user_version = 3;

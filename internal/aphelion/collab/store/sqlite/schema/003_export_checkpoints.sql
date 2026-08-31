CREATE TABLE IF NOT EXISTS export_checkpoints (
	checkpoint_id TEXT PRIMARY KEY,
	document_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	idempotency_key TEXT NOT NULL,
	revision INTEGER NOT NULL CHECK (revision >= 0),
	map_hash TEXT NOT NULL CHECK (length(map_hash) = 64),
	requested_by TEXT NOT NULL,
	created_at TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'rejected')),
	artifact_hash TEXT,
	verifier TEXT,
	verifier_version TEXT,
	diagnostic_code TEXT,
	completed_at TEXT,
	UNIQUE (document_id, idempotency_key),
	FOREIGN KEY (document_id) REFERENCES documents(document_id) ON DELETE CASCADE,
	CHECK (
		(status = 'pending' AND artifact_hash IS NULL AND verifier IS NULL AND verifier_version IS NULL AND diagnostic_code IS NULL AND completed_at IS NULL) OR
		(status = 'accepted' AND artifact_hash IS NOT NULL AND verifier IS NOT NULL AND verifier_version IS NOT NULL AND diagnostic_code IS NULL AND completed_at IS NOT NULL) OR
		(status = 'rejected' AND artifact_hash IS NULL AND verifier IS NOT NULL AND verifier_version IS NOT NULL AND diagnostic_code IS NOT NULL AND completed_at IS NOT NULL)
	)
);

PRAGMA user_version = 3;

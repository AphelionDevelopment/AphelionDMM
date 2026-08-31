CREATE TABLE collaboration_export_checkpoints (
	checkpoint_id TEXT PRIMARY KEY,
	document_id TEXT NOT NULL REFERENCES collaboration_documents(document_id) ON DELETE CASCADE,
	session_id TEXT NOT NULL,
	idempotency_key TEXT NOT NULL,
	revision BIGINT NOT NULL CHECK (revision >= 0),
	map_hash CHAR(64) NOT NULL,
	requested_by TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'rejected')),
	artifact_hash CHAR(64),
	verifier TEXT,
	verifier_version TEXT,
	diagnostic_code TEXT,
	completed_at TIMESTAMPTZ,
	UNIQUE (document_id, idempotency_key),
	CHECK (
		(status = 'pending' AND artifact_hash IS NULL AND verifier IS NULL AND verifier_version IS NULL AND diagnostic_code IS NULL AND completed_at IS NULL) OR
		(status = 'accepted' AND artifact_hash IS NOT NULL AND verifier IS NOT NULL AND verifier_version IS NOT NULL AND diagnostic_code IS NULL AND completed_at IS NOT NULL) OR
		(status = 'rejected' AND artifact_hash IS NULL AND verifier IS NOT NULL AND verifier_version IS NOT NULL AND diagnostic_code IS NOT NULL AND completed_at IS NOT NULL)
	)
);

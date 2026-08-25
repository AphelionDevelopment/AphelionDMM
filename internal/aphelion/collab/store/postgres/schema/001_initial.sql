CREATE TABLE collaboration_documents (
	document_id TEXT PRIMARY KEY,
	snapshot JSONB NOT NULL,
	snapshot_revision BIGINT NOT NULL CHECK (snapshot_revision >= 0),
	snapshot_hash CHAR(64) NOT NULL,
	current_revision BIGINT NOT NULL CHECK (current_revision >= snapshot_revision),
	current_hash CHAR(64) NOT NULL
);

CREATE TABLE collaboration_operations (
	document_id TEXT NOT NULL REFERENCES collaboration_documents(document_id) ON DELETE CASCADE,
	operation_id TEXT NOT NULL UNIQUE,
	revision BIGINT NOT NULL CHECK (revision > 0),
	accepted JSONB NOT NULL,
	map_hash CHAR(64) NOT NULL,
	PRIMARY KEY (document_id, revision)
);

CREATE TABLE collaboration_revision_hashes (
	document_id TEXT NOT NULL REFERENCES collaboration_documents(document_id) ON DELETE CASCADE,
	revision BIGINT NOT NULL CHECK (revision >= 0),
	map_hash CHAR(64) NOT NULL,
	PRIMARY KEY (document_id, revision)
);

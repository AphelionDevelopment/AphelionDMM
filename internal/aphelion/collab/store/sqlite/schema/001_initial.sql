CREATE TABLE IF NOT EXISTS documents (
	document_id TEXT PRIMARY KEY,
	snapshot BLOB NOT NULL,
	snapshot_revision INTEGER NOT NULL,
	snapshot_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operations (
	document_id TEXT NOT NULL,
	operation_id TEXT NOT NULL,
	revision INTEGER NOT NULL,
	accepted BLOB NOT NULL,
	map_hash TEXT NOT NULL,
	PRIMARY KEY (document_id, operation_id),
	UNIQUE (document_id, revision),
	FOREIGN KEY (document_id) REFERENCES documents(document_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS operations_replay
	ON operations(document_id, revision);

PRAGMA user_version = 1;

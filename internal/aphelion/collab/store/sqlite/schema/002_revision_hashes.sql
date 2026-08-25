CREATE TABLE IF NOT EXISTS revision_hashes (
	document_id TEXT NOT NULL,
	revision INTEGER NOT NULL,
	map_hash TEXT NOT NULL,
	PRIMARY KEY (document_id, revision),
	FOREIGN KEY (document_id) REFERENCES documents(document_id) ON DELETE CASCADE
);

INSERT OR IGNORE INTO revision_hashes(document_id, revision, map_hash)
	SELECT document_id, snapshot_revision, snapshot_hash FROM documents;

INSERT OR IGNORE INTO revision_hashes(document_id, revision, map_hash)
	SELECT document_id, revision, map_hash FROM operations;

PRAGMA user_version = 2;

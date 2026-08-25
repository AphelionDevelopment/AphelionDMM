CREATE TABLE collaboration_hosted_sessions (
	session_id TEXT PRIMARY KEY,
	document_id TEXT NOT NULL UNIQUE REFERENCES collaboration_documents(document_id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE collaboration_hosted_members (
	session_id TEXT NOT NULL REFERENCES collaboration_hosted_sessions(session_id) ON DELETE CASCADE,
	issuer TEXT NOT NULL,
	subject TEXT NOT NULL,
	actor_id TEXT NOT NULL,
	display_name TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('viewer', 'editor', 'owner')),
	disabled BOOLEAN NOT NULL DEFAULT FALSE,
	PRIMARY KEY (session_id, issuer, subject),
	UNIQUE (session_id, actor_id)
);

CREATE TABLE collaboration_hosted_invitations (
	token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
	session_id TEXT NOT NULL REFERENCES collaboration_hosted_sessions(session_id) ON DELETE CASCADE,
	role TEXT NOT NULL CHECK (role IN ('viewer', 'editor')),
	created_by_actor_id TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	redeemed_at TIMESTAMPTZ
);

CREATE INDEX collaboration_hosted_invitations_session_idx
	ON collaboration_hosted_invitations(session_id, expires_at);

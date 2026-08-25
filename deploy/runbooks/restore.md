# Collaboration restore runbook

## Decision and containment

1. Stop new joins and writes at the edge. Preserve the failed database and relevant logs; do not restore over it.
2. Record the requested recovery point, last known acknowledged revision, incident identifier, backup SHA-256, and operator.
3. Create a new empty PostgreSQL database with the approved server major version. Restore into it with a separate restore identity.

## Restore

Provide libpq secrets through the deployment secret mechanism. Do not put credentials in arguments or output.

```text
sha256sum --check <archive>.sha256
pg_restore --dbname="$PGDATABASE" --no-owner --no-privileges --exit-on-error <archive>
```

Run the application migration command only when the restored migration version is within the documented compatibility window. Never downgrade a database in place.

## Integrity checks

Run these read-only queries using the configured collaboration schema:

```sql
SELECT COALESCE(MAX(version), 0) AS migration_version FROM collaboration_schema_migrations;

SELECT document_id, snapshot_revision, current_revision
FROM collaboration_documents
WHERE snapshot_revision > current_revision;

SELECT o.document_id, o.revision
FROM collaboration_operations AS o
LEFT JOIN collaboration_revision_hashes AS h
  ON h.document_id = o.document_id AND h.revision = o.revision
WHERE h.document_id IS NULL;

SELECT document_id, revision, COUNT(*)
FROM collaboration_operations
GROUP BY document_id, revision
HAVING COUNT(*) <> 1;
```

The second, third, and fourth queries must return no rows. Start one application instance against the restored database, load every retained document, replay from its snapshot, and compare the reconstructed current revision/hash with `collaboration_documents.current_revision/current_hash` before reopening traffic.

## Recovery point acceptance

- Confirm whether the selected archive is before or after the incident operation and record the visible revision.
- Compare the restored revision against the last acknowledged revision from service evidence. Any acknowledged revision absent after restore is a severity-one data-loss incident; do not reopen the pilot.
- Record total restoration time against the 60-minute target and attach archive and final map hashes.

Switch the service to the restored database through immutable deployment configuration. Retain the failed database and restored validation evidence until incident closure.


# Collaboration backup runbook

## Targets and ownership

- Service owner: AphelionDMM hosted-service operator.
- Backup owner: database operator for the deployment.
- Pilot recovery point target: at most five minutes of backup exposure for data not already protected by PostgreSQL transaction durability.
- Pilot recovery time target: restore service within 60 minutes.
- Acknowledged operations are committed transactionally and are not allowed to depend on an asynchronous application buffer.

## Preconditions

1. Confirm `/v1/health/ready` is healthy and record the deployed build, protocol matrix, PostgreSQL server version, and migration version.
2. Use a dedicated least-privilege backup identity with `CONNECT` and read access to the collaboration schema. It must not own the database or have application write privileges.
3. Supply libpq connection values through the deployment secret mechanism (`PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, and `PGPASSFILE` or `PGPASSWORD`). Never put a password or DSN containing a password in command arguments, logs, filenames, or shell history.
4. Write the archive to an encrypted destination outside the service container writable layer.

## Logical backup

```text
pg_dump --format=custom --no-owner --no-privileges --file=<staged-archive> "$PGDATABASE"
pg_restore --list <staged-archive>
sha256sum <staged-archive> > <staged-archive>.sha256
```

Move the archive and checksum into immutable backup storage only after both commands succeed. Record start/end time, PostgreSQL version, application build, schema migration version, archive byte size, SHA-256, and retention class. Redact command output before attaching it to an incident or change record.

## Schedule and retention

- Run a logical backup before every migration and deployment that changes persistence behavior.
- During the pilot, take scheduled logical backups frequently enough to satisfy the five-minute exposure target, or use managed PostgreSQL continuous archiving/PITR with an equivalent tested recovery point.
- Retain at least one verified pre-migration backup until the compatibility window closes and rollback is no longer possible.
- Encrypt backups in transit and at rest; restrict read/delete access separately from application credentials.

## Verification

The backup is not accepted until it is restored into an empty isolated database and the revision/hash integrity checks from `restore.md` pass. The local automated rehearsal is `TestLogicalBackupRestoresExactRevisionAndHashBeforeAndAfterOperation`.


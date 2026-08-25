# Collaboration migration runbook

## Compatibility policy

- Migrations are forward-only. A failed migration is rolled forward unless the old application and old database are both restored from the verified pre-migration backup.
- The current hosted service is single-replica. Upgrade it with a stop-then-start replacement only after `apheliondmm-compat` accepts the target build and both builds support the same database migration version.
- Schema changes must remain compatible with the previous application build for the rollback window. Destructive column/table changes require a later cleanup release after rollback to the old build is no longer allowed.
- Concurrent old/new application replicas are forbidden until accepted operations have a tested cross-instance fanout mechanism.

## Procedure

1. Record the current application build, protocol matrix, migration version, document count, current revisions, and readiness state.
2. Complete and verify the pre-migration backup using `backup.md`.
3. Run the compatibility command for the intended old/new pair. Stop if it rejects either direction.
4. Quiesce migrations to one operator/job. Application startup also takes the PostgreSQL advisory migration lock; never bypass it.
5. Run the new binary's migration/startup against an isolated restored copy first. Execute the integrity queries from `restore.md`.
6. In an isolated restored environment, start the new build as the only replica. Confirm readiness, replay/hash integrity, and zero authorization failures.
7. Stop the production replica, start the new replica, and confirm readiness before reopening traffic. Preserve the old image and backup for the rollback window.

## Failure decision

- Before any new-version write: the canary may be removed and the old build retained.
- After a compatible additive migration and new-version write: fix forward with a tested migration/application patch.
- After an incompatible or destructive change: stop traffic and restore both the old build and the verified pre-migration database into a new database. Never attempt ad-hoc reverse SQL on production data.

Record the migration lock duration, migration version before/after, canary evidence, integrity results, rollback/roll-forward decision, and approver.

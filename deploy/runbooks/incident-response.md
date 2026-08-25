# Hosted collaboration incident response

## Severity-one triggers

- acknowledged-operation loss or canonical-hash divergence;
- authorization bypass, cross-session access, token leakage, or forged identity;
- unrecoverable protocol/schema incompatibility;
- backup disclosure or inability to restore inside the target;
- updater or release-signing compromise.

## Immediate actions

1. Stop pilot expansion. Disable new joins and writes at the trusted edge while preserving health access for operators.
2. Revoke affected OIDC sessions, invitation credentials, deployment secrets, and release credentials as applicable.
3. Preserve database, immutable logs, build digest, compatibility matrix, deployment configuration hash, and last acknowledged revision/hash evidence. Do not copy map content into tickets or chat.
4. Assign incident commander, application owner, database owner, identity owner, and communications owner.
5. For possible data loss, restore into a new isolated database using `restore.md`; never overwrite primary evidence.

## Recovery gates

Traffic remains closed until the root cause has a regression test or operational control, restored/current hashes are reconciled, authorization revocation is verified, and rollback has been rehearsed. A severity-one event requires explicit human approval before the pilot resumes.

Record detection/recovery times, affected sessions/builds, p50/p95/p99 acknowledgement latency, reconnect rate, conflicts, store health, credential actions, recovery point, final hashes, and follow-up owners. Redact tokens, headers, DSNs, map content, and personal identity claims beyond immutable internal actor identifiers.


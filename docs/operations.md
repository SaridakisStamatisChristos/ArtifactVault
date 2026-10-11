# Operations, recovery, and rollback

## Backups

For a coherent offline backup, stop the service, copy `artifactvault.db` plus the complete `storage/objects` tree, then restart. If online backup is required, use a SQLite-aware snapshot method and a filesystem snapshot with an externally coordinated consistency point; the current product does not claim a live cross-resource snapshot protocol.

## Restart recovery

At startup ArtifactVault performs conservative recovery:

1. stale files in `staging/` older than `ARTIFACTVAULT_STAGING_TTL` are removed;
2. interrupted GC entries in `trash/` are restored if metadata still exists, otherwise removed;
3. old files in the canonical object tree that have no DB row are moved to `lost+found/` rather than silently imported.

DB rows whose physical file was externally removed are not repaired. `verify` reports the loss.

A partial recovery failure is retained as service state. The process may remain live for diagnosis, but `/health/ready` returns 503 until a subsequent recovery pass completes cleanly; operators should not route normal traffic to an instance reporting this condition.

## Corruption

Run `artifactvault verify SHA`. Never overwrite a corrupted object in place. Preserve the DB and corrupted bytes for diagnosis. Restore the exact object from a trusted backup and verify it before reopening normal access. Verification itself performs no repair.

## Garbage collection

Default grace is controlled by `ARTIFACTVAULT_GC_GRACE`. A zero grace is allowed only when explicitly requested/configured; it does not bypass the active-reference check. GC's trash rename plus DB transaction is designed for restart-safe recovery.

## Database migrations

`artifactvault migrate up` applies committed migrations. `artifactvault migrate down` rolls back one version when the migration includes a down file. CI rehearses the complete migration chain down to version 0 and back to the current version.

Before upgrading a production-like deployment, back up DB and object storage. If application rollback also requires a schema rollback, stop the new binary, run the matching `migrate down`, then start the previous binary. Never downgrade a schema while a newer process is serving requests.

## Readiness incidents

A 503 from `/health/ready` means startup recovery is incomplete/failed, SQLite is unavailable, or writable staging storage is unavailable. `/health/live` may remain 200 because process liveness is intentionally not dependency/readiness safety.

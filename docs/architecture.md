# Architecture

## Topology lock

ArtifactVault intentionally has four components: CLI, HTTP API, storage service, and persistence adapter. The CLI depends only on the HTTP contract. HTTP owns request validation/transport semantics. The storage service owns content identity, lifecycle locks, integrity, and GC. The SQLite adapter owns durable metadata and migrations. No component is permitted to derive filesystem paths from unvalidated user paths.

## Ingestion state machine

`request body -> staging file -> fsync -> SHA/size known -> lifecycle lock -> CAS finalization -> SQLite transaction -> response`

A staging file has no artifact identity. The final path is derived solely from a validated lowercase SHA-256. The server uses a same-filesystem hard link from staging to the final path, so an existing destination is never overwritten. A successful finalization requires the object file and containing directory durability syncs to succeed. If the final object already exists it is recomputed and compared before being reused. Metadata is inserted only after finalized bytes exist.

If metadata insertion fails after this process created a new final object, the object is removed before the upload returns failure. A crash in the narrow finalization-before-commit window can leave an untracked finalized object; startup recovery never invents metadata and eventually quarantines old untracked objects to `lost+found`.

## Physical objects and logical records

`objects.sha256` is the physical identity. `artifact_records` are logical retention/metadata records. Many records may reference one object. Tags belong to logical records. A duplicate upload therefore reuses bytes while preserving independent filename/tags/description provenance.

Deletion marks one logical record deleted. If a hash has several active records and the caller omits `record_id`, the request fails rather than guessing. `objects.unreferenced_at` is set only when the active reference count reaches zero.

## GC crash safety

GC selects only objects whose `unreferenced_at` is older than the grace cutoff. Under the lifecycle write lock it rechecks active retention, renames the object into `trash/`, deletes logical/database state transactionally, then unlinks trash. If the DB transaction fails, it renames the object back. If the process crashes in the middle, startup recovery restores trash when the DB row still exists, or removes trash when metadata deletion committed.

## Concurrency contract

One ArtifactVault server process may handle many clients. Upload bodies stage concurrently; finalization/metadata lifecycle transitions are serialized briefly by a process lifecycle lock. Content reads hold a read lease for the duration of integrity pre-verification plus `ServeContent`, preventing delete/GC from racing an active read. Before any download bytes are sent, the physical file is re-hashed and compared with its SHA-256 identity; same-size corruption is therefore rejected rather than streamed. SQLite is opened in full-mutex mode and the adapter serializes use of its single connection.

The repository does **not** claim correctness for multiple independent server processes sharing one SQLite database/storage root. A future multi-process mode would require an inter-process lifecycle lock or a transactional object-store protocol.

## Durability contract

Success is returned only after staged bytes are fsynced, finalized, the containing directory sync succeeds, and a `synchronous=FULL` SQLite transaction records the object/reference. This is a local-filesystem durability model; storage hardware/controller guarantees remain outside application control.

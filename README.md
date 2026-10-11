# ArtifactVault

ArtifactVault is a local-first, content-addressed artifact storage service for software teams. Exact binary bytes define identity: `SHA256(exact bytes)`. It exposes a REST API and a single-binary CLI while persisting metadata in SQLite and artifact bytes in a filesystem CAS.

## Guarantees

- Uploads stream through a fixed 64 KiB buffer; file size does not determine process memory usage.
- Bytes are SHA-256 hashed while streaming to a staging file, `fsync`ed, then finalized into a hash-derived path before metadata is committed.
- Identical bytes share one physical object. Each upload may create a distinct logical metadata record that retains the same object.
- A failed upload does not create a valid logical artifact record.
- Downloads are served only for active logical retention records, are re-hashed before any bytes are returned, and support HTTP byte ranges.
- Integrity verification recomputes SHA-256 and reports `verified`, `missing_physical_object`, `corrupted_object`, or `metadata_inconsistency`; it never silently repairs corruption.
- Logical deletion never directly removes bytes. Garbage collection removes an object only after all logical records are deleted and the grace period has elapsed.
- A read lease blocks destructive lifecycle operations while content is being served.

ArtifactVault deliberately targets a **single service process over one local filesystem and one SQLite database**. Multiple independent server processes sharing the same database/storage root are not a declared consistency target.

## Quick start

The simplest path is Docker Compose:

```bash
docker compose up --build
```

The API then listens on `http://127.0.0.1:8080`.

Native development requires Go 1.23+, a C compiler, `pkg-config`, and SQLite development headers (`libsqlite3-dev` on Debian/Ubuntu):

```bash
make test
make build
ARTIFACTVAULT_DB_PATH=./data/artifactvault.db \
ARTIFACTVAULT_STORAGE_ROOT=./data/storage \
./bin/artifactvault serve
```

## CLI

The CLI defaults to `http://127.0.0.1:8080`; set `ARTIFACTVAULT_URL` to change it.

```bash
artifactvault upload --tag benchmark --description "run 42" results.tar.zst
artifactvault list --tag benchmark --limit 25
artifactvault get --output restored.tar.zst <SHA256>
artifactvault verify <SHA256>
artifactvault delete <SHA256>
artifactvault gc
```

If several active logical records retain the same SHA-256, deletion by hash is intentionally rejected as ambiguous. Select the record explicitly:

```bash
artifactvault delete --record <RECORD_UUID> <SHA256>
```

To override the configured grace period for an authorized manual collection:

```bash
artifactvault gc --grace 0s
```

## REST API

The committed and runtime-served OpenAPI 3.1 document is [`api/openapi.json`](api/openapi.json) and is available at `GET /openapi.json`. Core routes:

- `POST /artifacts` — raw streaming request body; metadata in `X-Artifact-Filename`, `X-Artifact-Tags`, `X-Artifact-Description`, and `Content-Type`.
- `GET /artifacts` — keyset pagination plus tag/creation-time filters.
- `GET /artifacts/{sha256}` — physical identity plus active logical metadata records.
- `GET /artifacts/{sha256}/content` — exact bytes, including HTTP Range support.
- `DELETE /artifacts/{sha256}` — logical deletion only.
- `POST /artifacts/{sha256}/verify` — non-repairing integrity verification.
- `POST /gc` — grace-aware physical collection.
- `GET /health/live` and `GET /health/ready` — deliberately distinct health semantics.
- `GET /metrics` — basic Prometheus text counters.

See [`docs/api.md`](docs/api.md) for error and metadata semantics.

## Storage layout

```text
<data>/
  artifactvault.db
  storage/
    staging/      # in-progress uploads; never valid artifacts
    objects/aa/bb/<sha256>
    trash/        # crash-safe GC handoff
    lost+found/   # old finalized orphans with no metadata
```

The database uses WAL mode, `synchronous=FULL`, foreign keys, and versioned migrations in `internal/db/migrations/`.

## Health and operations

`/health/live` proves only that the HTTP process responds. `/health/ready` additionally verifies SQLite access and an actual create/write/fsync/remove probe in staging storage. If a required dependency is unusable, readiness returns 503 while liveness can remain 200.

Structured logs are JSON in normal server execution. Recovery, backup, restore, corruption, GC, and rollback procedures are in [`docs/operations.md`](docs/operations.md).

## Development and evidence

```bash
make fmt
make vet
make test
make race
make build
make manifest
```

CI executes formatting, vet, race-enabled tests, binary build, migration up/down/up rehearsal, and a Docker image build. R7 requirement, assumption, claim, and failure/repair ledgers live under [`evidence/`](evidence/).

## Scope and trust boundary

ArtifactVault v0.1 is intended for trusted local/internal deployment. Authentication/authorization, multi-tenancy, Kubernetes, distributed locking, object-store backends, and internet-facing hardening are **not** silently implied. See [`SECURITY.md`](SECURITY.md) for explicit N/A rationale and the controls that are implemented.

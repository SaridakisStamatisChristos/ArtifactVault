# Verification

ArtifactVault separates repository-level verification from operational claims. A green CI run demonstrates that the checked revision satisfies the automated code, migration, packaging, and container checks below. It is not evidence that a deployment has been load-tested, penetration-tested, or proven under every host/filesystem failure mode.

## Automated gates

The GitHub Actions workflow requires:

- `gofmt` cleanliness;
- `go vet ./...`;
- the complete Go test suite with the race detector and cache disabled;
- a clean binary build;
- migration rehearsal from the current schema down to version 0 and back to the current version;
- Docker Compose configuration validation;
- Docker image build;
- container startup followed by a `/health/ready` probe.

## Behavior covered by repository tests

The test suite exercises the product's critical invariants, including:

- exact-byte SHA-256 identity, including empty content and one-byte changes;
- bounded-buffer streaming ingestion and configured maximum artifact size;
- sequential and concurrent physical deduplication;
- incomplete/failed upload cleanup;
- persistent metadata and reversible migrations;
- lookup, list filters, stable pagination, downloads, and HTTP byte ranges;
- pre-download re-hashing and corruption refusal;
- explicit verification states for missing, corrupt, and metadata-inconsistent objects;
- logical deletion and retained-object protection;
- grace-aware garbage collection and active-read lifecycle locking;
- restart recovery for stale staging, interrupted GC, and orphaned finalized objects;
- readiness behavior when recovery, SQLite, or writable storage is unsafe;
- OpenAPI route consistency with runtime handlers;
- CLI core flows and atomic output-file replacement on failed downloads.

## Claims intentionally not made

The repository does not claim:

- correctness for multiple independent ArtifactVault server processes sharing one SQLite database/storage root;
- internet-facing security without an external trust boundary;
- production-proven availability or durability across arbitrary hardware/power-loss scenarios;
- sustained throughput or latency targets;
- multi-tenant isolation;
- distributed replication or high availability.

Those properties require separate evidence and, in several cases, a different architecture.

## Local verification

Run the primary checks with:

```bash
make fmt
make vet
make test
make race
make build
```

For packaging validation:

```bash
docker compose config
docker build -t artifactvault:local .
```

The canonical API contract is `api/openapi.json`; tests ensure its method/path set stays aligned with the runtime route declaration.

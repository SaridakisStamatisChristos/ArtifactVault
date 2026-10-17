# Development

Native development requires Go 1.23+, a C toolchain, `pkg-config`, and SQLite development headers. The Go module has no third-party Go dependencies; the persistence adapter links the host SQLite library directly via cgo.

## Local loop

```bash
make fmt
make vet
make test
make race
make build
```

The automated tests create isolated temporary databases and storage roots. Concurrency, corruption, migration, readiness, recovery, and OpenAPI-contract tests are part of the repository's behavior contract; repair regressions rather than weakening those checks to obtain a green build.

Docker is the dependency-contained path:

```bash
docker compose up --build
```

## Contract changes

If HTTP routes or semantics change, update `api/openapi.json` in the same change. If persistent schema changes, add a versioned migration and a safe rollback where possible. If storage lifecycle behavior changes, add failure-path and concurrency coverage, not only happy-path tests.

Keep product documentation aligned with observable behavior. `README.md`, `docs/api.md`, `docs/architecture.md`, `docs/operations.md`, `SECURITY.md`, and `.env.example` are the public repository contract surfaces.

See [`verification.md`](verification.md) for the CI and verification boundary.

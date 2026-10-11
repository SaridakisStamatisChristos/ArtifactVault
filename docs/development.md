# Development

Native prerequisites are Go 1.23+, a C toolchain, `pkg-config`, and SQLite development headers. No third-party Go module is required; the persistence adapter links the host SQLite library directly via cgo.

Typical loop:

```bash
make fmt
make vet
make test
make race
make build
```

The automated tests create isolated temporary databases and storage roots. Do not weaken concurrency, corruption, migration, readiness, or OpenAPI-drift tests to obtain a green build; repair the implementation or revise the explicit contract with evidence.

Docker is the recommended dependency-contained local path: `docker compose up --build`.

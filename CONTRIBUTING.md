# Contributing

Changes should preserve the content identity invariant `SHA256(exact bytes)` and the rule that metadata may only claim storage after finalized bytes exist. New lifecycle behavior requires tests for failure and concurrency boundaries, not only happy paths.

Before opening a change, run `make fmt`, `make vet`, `make race`, and `make build`. If API routes or semantics change, update `api/openapi.json` in the same change. If persistent schema changes, add a versioned migration and rollback when safe.

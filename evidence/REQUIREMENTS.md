# R7 requirement evidence matrix

States describe the current revision in the execution environment used to build it. `VERIFIED` means directly executed evidence; committed CI configuration alone is not evidence of a CI run.

| ID | Hard requirement | Implementation / evidence | State |
|---|---|---|---|
| R-01 | Identity = SHA-256 of exact bytes; one-byte change changes identity | incremental `crypto/sha256`; round-trip, empty, explicit one-byte-change and corruption tests | VERIFIED |
| R-02 | Streaming upload with bounded memory, size/hash during ingest | 64 KiB `io.CopyBuffer`; 16 MiB generated-stream test | VERIFIED |
| R-03 | Staging then durable finalization; no success before complete finalization | staging + file `Sync` + hard-link CAS finalize + mandatory directory `Sync` + DB transaction; failure injection test | VERIFIED |
| R-04 | Physical dedup including concurrent identical upload | one hash path; existing object rehashed; 12-way duplicate test | VERIFIED |
| R-05 | Concurrent distinct uploads remain correct | 10-way distinct upload test | VERIFIED |
| R-06 | Disconnect/failed upload cannot create valid record | failing-reader test, staging leak assertion | VERIFIED |
| R-07 | Persistent metadata, real migrations | SQLite + versioned up/down SQL; migration up/down/up test | VERIFIED |
| R-08 | Lookup/download/list/pagination/tag/time filters | store/API implementation; HTTP filter tests + stable keyset pagination test | VERIFIED |
| R-09 | HTTP Range retrieval | `http.ServeContent`; 206/416 tests | VERIFIED |
| R-10 | Downloaded bytes match content identity | exact round-trip + pre-download re-hash; same-size corrupted object refused by store and HTTP tests | VERIFIED |
| R-11 | Verification distinguishes verified/missing/corrupt/metadata inconsistency without repair | explicit result states; independent mutation tests | VERIFIED |
| R-12 | Safe logical deletion with shared physical object | per-upload logical records; ambiguous delete conflict; retained-object tests | VERIFIED |
| R-13 | Explicit GC with configurable grace; never delete retained data | unreferenced timestamp + recheck + trash transaction; grace/retention tests | VERIFIED |
| R-14 | GC/read concurrency safety | lifecycle RW lock; active-read blocks delete+GC until lease release | VERIFIED |
| R-15 | Precise consistency, no fake exactly-once guarantee | architecture docs describe one-process model and duplicate logical records | VERIFIED (documentation + tests) |
| R-16 | REST API and defined malformed/error behavior | HTTP handlers + OpenAPI; malformed hash, metadata, time, range tests | VERIFIED |
| R-17 | Committed OpenAPI does not silently drift | embedded committed JSON + route-equivalence test; runtime handler registration is derived from the same route declaration and panics on omissions | VERIFIED |
| R-18 | Required CLI commands and README commands work | CLI implementation + in-process CLI smoke covering upload/get/list/verify/delete/gc | VERIFIED |
| R-19 | Liveness/readiness separated; readiness checks required dependencies | live process probe vs DB + storage write/fsync probe; closed-DB HTTP test | VERIFIED |
| R-20 | Structured logs/basic metrics | JSON `slog`, Prometheus text counters | PARTIAL (code exercised; log/metrics semantics not load-tested) |
| R-21 | Configuration/env and no embedded secrets | config parser + `.env.example`; source inspection | VERIFIED |
| R-22 | Path traversal/arbitrary read/staging escape defenses | strict 64-hex hash, server-created temp files, derived CAS path | VERIFIED |
| R-23 | Bounded metadata and safe errors | configured limits + API tests; generic 500s | VERIFIED |
| R-24 | Restart/recovery behavior | `Recover` + stale staging/orphan tests + simulated interrupted-GC trash restoration and re-verification | PARTIAL (representative crash state executed; process-kill fault injection not executed) |
| R-25 | Serious automated tests without weakened invariants | full Go suite locally green, including verification concurrent with uploads and metadata persistence across reopen | VERIFIED |
| R-26 | Dockerfile/Compose/local developer path | Docker image built successfully in GitHub Actions run 38117727427; follow-up gate validates Compose and boots the image through readiness | VERIFIED externally |
| R-27 | CI | GitHub Actions run 38117727427: formatting, vet, race tests, build, migration rehearsal, and container build completed successfully | VERIFIED externally |
| R-28 | Release/rollback/recovery guidance | operations docs | VERIFIED as documentation |
| R-29 | Kubernetes only if valuable | explicitly excluded for single-node SQLite/local-storage target | N/A |
| R-30 | Performance claims only with evidence | no throughput/latency claim; bounded-buffer property tested | VERIFIED |
| R-31 | README, architecture, development, API, operations, contribution, security docs | repository documentation set | VERIFIED by file inspection |
| R-32 | Real file manifest/checksums | `scripts/manifest.sh` + generated `evidence/MANIFEST.sha256` | VERIFIED after generation |

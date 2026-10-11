# R7 assumption ledger

| ID | Statement | Criticality | Confidence | Affected surface | Invalidation trigger | Final state |
|---|---|---:|---:|---|---|---|
| A-01 | Initial deployment is one ArtifactVault service process using one local filesystem and SQLite DB. | HIGH | High | concurrency, GC, durability | requirement for multi-process/HA writers | CLOSED / documented |
| A-02 | Trusted internal/local clients are sufficient; application auth is intentionally absent. | HIGH | High | security boundary | internet-facing or multi-tenant deployment | CLOSED / documented |
| A-03 | Host filesystem supports hard links for the primary atomic finalize path. | MEDIUM | High on Linux/local FS | ingestion | deployment on FS without hard links | CONDITIONAL; finalize errors rather than unsafe overwrite |
| A-04 | Go 1.23+, cgo toolchain, and system SQLite are acceptable native prerequisites; Docker is the dependency-contained path. | MEDIUM | High | developer setup | pure-static/no-cgo requirement | CLOSED / documented |
| A-05 | No application-level maximum artifact size is required by default; operator may configure one. | LOW | High | ingestion | explicit mandatory size ceiling | CLOSED / configurable |

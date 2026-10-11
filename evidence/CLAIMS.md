# R7 claim ledger

| Claim | State | Evidence / limitation |
|---|---|---|
| Artifact identity is exact-byte SHA-256 | VERIFIED locally | streaming upload + round-trip/empty/one-byte-change/corruption tests |
| Upload memory is bounded independently of artifact size | VERIFIED locally for implementation property | 64 KiB copy buffer and 16 MiB generated-stream test confirms max read request <=64 KiB; no throughput claim |
| Duplicate physical data is eliminated in one service process | VERIFIED locally | sequential + 12-way concurrent duplicate tests leave one object file |
| GC preserves active retention and grace | VERIFIED locally | retained-object, ambiguity, grace, active-read lease, deletion tests |
| Verification detects missing/corrupt/metadata-inconsistent objects | VERIFIED locally | independent filesystem/DB mutation tests |
| REST and CLI core flows work | VERIFIED locally | HTTP and CLI smoke tests; corrupted download rejected before serving bytes |
| Docker image is buildable | UNVERIFIED locally | Docker unavailable in execution environment; CI lane added |
| Production-ready | UNVERIFIED | local correctness evidence is not production observation/load/penetration evidence |
| High throughput / low latency | UNVERIFIED / no claim | no benchmark performed |

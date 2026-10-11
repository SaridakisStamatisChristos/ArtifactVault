# R7 claim ledger

| Claim | State | Evidence / limitation |
|---|---|---|
| Artifact identity is exact-byte SHA-256 | VERIFIED locally | streaming upload + round-trip/empty/one-byte-change/corruption tests |
| Upload memory is bounded independently of artifact size | VERIFIED locally for implementation property | 64 KiB copy buffer and 16 MiB generated-stream test confirms max read request <=64 KiB; no throughput claim |
| Duplicate physical data is eliminated in one service process | VERIFIED locally | sequential + 12-way concurrent duplicate tests leave one object file |
| GC preserves active retention and grace | VERIFIED locally | retained-object, ambiguity, grace, active-read lease, deletion tests |
| Verification detects missing/corrupt/metadata-inconsistent objects | VERIFIED locally | independent filesystem/DB mutation tests |
| REST and CLI core flows work | VERIFIED locally | HTTP and CLI smoke tests; corrupted download rejected before serving bytes |
| Docker image is buildable | VERIFIED externally | GitHub Actions run 38117727427 container job 114405896309 completed successfully on implementation commit `4b1b99d`; follow-up CI also boots the image and probes readiness |
| GitHub CI gates execute successfully | VERIFIED externally | GitHub Actions run 38117727427 test job 114405896167 passed formatting, vet, race tests, build, and migration rehearsal |
| Production-ready | UNVERIFIED | correctness and packaging evidence is not production observation/load/penetration evidence |
| High throughput / low latency | UNVERIFIED / no claim | no benchmark performed |

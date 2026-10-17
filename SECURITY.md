# Security and trust boundary

ArtifactVault is designed for trusted local and internal networks. The current product boundary does not include application authentication or authorization, multi-tenancy, TLS termination, distributed writers, rate limiting, or enterprise identity integration. Exposing the service directly to untrusted networks is unsupported.

Implemented controls include strict SHA-256 syntax validation, hash-derived filesystem paths, server-created staging paths, bounded metadata, safe JSON errors, no user-supplied filesystem path reads, no embedded secrets, bounded list pages, request-header limits, corruption detection, conservative recovery, and separate liveness/readiness semantics.

The default server bind is loopback-only. The container listens on `0.0.0.0:8080` inside the container, while the supplied Docker Compose mapping publishes the service only on host loopback (`127.0.0.1:8080`). Operators who intentionally publish ArtifactVault on another interface are expanding the trust boundary and must provide appropriate network controls and, when needed, TLS and authentication in front of the service.

Filesystem permissions, host access, backup protection, and storage-device security remain deployment responsibilities.

ArtifactVault intentionally supports one service process over one SQLite database and one local storage root. Multiple independent server processes sharing the same database/storage root are outside the supported consistency model.

Kubernetes is not part of the current architecture because it would add orchestration surface without solving a required single-node storage problem and could encourage unsupported multi-replica use over SQLite/local storage.

Report suspected vulnerabilities privately to the repository owner rather than publishing exploit details before coordination.

# Security and trust boundary

ArtifactVault v0.1 is for trusted local/internal networks. It has no authentication or authorization layer by design; exposing it directly to untrusted networks is unsupported. Enterprise identity, tenant isolation, rate limiting, TLS termination, and audit-policy integration are N/A for the declared deployment boundary, not implicit features.

Implemented controls include strict SHA-256 syntax validation, hash-derived filesystem paths, staging paths created by the server, metadata bounds, safe JSON errors, no user-supplied filesystem path reads, no embedded secrets, bounded list pages, request-header limits, corruption detection, and separation of liveness/readiness.

Filesystem permissions and host/container access remain deployment responsibilities. Report suspected vulnerabilities privately to the repository owner rather than publishing exploit details before coordination.

Kubernetes is N/A for the initial single-node/local-first architecture. It would add operational surface without solving a required deployment problem and could encourage unsupported multi-replica use over SQLite/local storage.

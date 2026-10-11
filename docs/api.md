# API semantics

`api/openapi.json` is the committed OpenAPI 3.1 contract and is served verbatim at `/openapi.json`. Automated tests compare its method/path set to the runtime route declaration.

Uploads use a raw binary body so large artifacts need no multipart buffering. The optional metadata headers are bounded by configuration. Filenames are metadata only and are never joined into a filesystem path. Tags are comma-separated for the HTTP surface and repeatable via the CLI.

List pagination uses an opaque keyset cursor over `(created_at, record_id)` with a maximum page size of 100. `created_after` and `created_before` must be RFC3339 timestamps. Tag filtering matches an exact tag.

Hashes must be exactly 64 hexadecimal characters and are normalized to lowercase. Invalid hashes are `400`. Missing/non-retained artifacts are `404`. Content is re-hashed before serving; an integrity mismatch returns `409 integrity_conflict` and no artifact bytes. Invalid/unsatisfiable byte ranges are handled by Go's `http.ServeContent` and return `416`.

Deletion is logical. With exactly one active logical record, `DELETE /artifacts/{sha256}` deletes that record. With several, the same request returns `409 ambiguous_delete`; provide `?record_id=<uuid>` to select one. Physical deletion happens only via GC.

Verification returns HTTP 200 when it successfully performs an integrity examination, even if the result reports corruption. Callers must inspect the `status` field: `verified`, `missing_physical_object`, `corrupted_object`, or `metadata_inconsistency`.

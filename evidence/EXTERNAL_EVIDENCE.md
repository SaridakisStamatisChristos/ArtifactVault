# R7 external evidence

## Implementation commit

- Commit: `4b1b99de3cd0d4547577aa27126b0d69f47ceade`
- GitHub Actions run: `38117727427` (CI run #1, push to `main`)
- Test job: `114405896167` — **success**
  - formatting: success
  - `go vet ./...`: success
  - `go test -race -count=1 ./...`: success
  - binary build: success
  - migration `up -> down -> up`: success
- Container job: `114405896309` — **success**
  - Docker image build: success

This evidence closes the execution environments that were unavailable in the synthesis container. It does not establish production deployment, sustained load behavior, penetration resistance, or hardware-level durability beyond the documented local-filesystem contract.

The evidence-closure follow-up CI additionally checks the committed SHA-256 manifest, validates Docker Compose, boots the built image, and requires `/health/ready` to succeed.

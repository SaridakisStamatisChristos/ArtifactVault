#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
out="evidence/MANIFEST.sha256"
tmp="${out}.tmp"
find . -type f \
  ! -path './.git/*' \
  ! -path './data/*' \
  ! -path './bin/*' \
  ! -path './evidence/MANIFEST.sha256' \
  ! -path './evidence/MANIFEST.sha256.tmp' \
  -print | LC_ALL=C sort | sed 's#^./##' | while IFS= read -r f; do sha256sum "$f"; done > "$tmp"
mv "$tmp" "$out"

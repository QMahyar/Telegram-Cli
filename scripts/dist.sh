#!/usr/bin/env bash
# Builds the cross-platform release binaries into dist/ and writes SHA256SUMS.
# Usage: scripts/dist.sh [version-tag]   (default: latest git tag, else 0.0.0-dev)
#
# The version tag is baked into the binary via ldflags
# (-X telegram-cli/internal/cli.version) so `telegram-cli version` reports
# the release. Asset names must match scripts/install.js:
#   telegram-cli-<os>-<arch>[.exe]
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo 0.0.0-dev)}"
LDFLAGS="-s -w -X telegram-cli/internal/cli.version=$VERSION"

PLATFORMS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

rm -rf dist
mkdir -p dist

for p in "${PLATFORMS[@]}"; do
  set -- $p
  os="$1"
  arch="$2"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  name="telegram-cli-$os-$arch$ext"
  echo "building $name (version $VERSION)"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" -o "dist/$name" ./cmd/telegram-cli
done

(cd dist && sha256sum telegram-cli-* > SHA256SUMS)
echo "dist ready:"
ls -1 dist

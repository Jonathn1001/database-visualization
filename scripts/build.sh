#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

DIST="internal/server/dist"

# 1. Frontend — clean stale assets but keep the .gitkeep placeholder, then build.
echo "→ Building React app..."
find "$DIST" -type f ! -name '.gitkeep' -delete
(cd web && npm ci && npm run build)

# 2. Bundle size budget (DESIGN_PLAN §14): embedded SPA must stay <= 2 MB raw.
BUNDLE_KB=$(du -sk "$DIST" | cut -f1)
if [ "$BUNDLE_KB" -gt 2048 ]; then
  echo "✗ Bundle size ${BUNDLE_KB}KB exceeds 2MB budget"
  exit 1
fi
echo "✓ Bundle size: ${BUNDLE_KB}KB"

# 3. Backend cross-compile (§21). Pure Go, CGO disabled.
PLATFORMS=("darwin/arm64" "darwin/amd64" "linux/amd64" "windows/amd64")
mkdir -p dist
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
for platform in "${PLATFORMS[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  output="dist/dbviz-${GOOS}-${GOARCH}"
  [ "$GOOS" = "windows" ] && output+=".exe"
  echo "→ Building $output"
  GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o "$output" ./main.go
done

# 4. Binary size check (§21.4): warn above 60 MB hard ceiling.
for f in dist/dbviz-*; do
  SIZE_MB=$(du -m "$f" | cut -f1)
  echo "  $f: ${SIZE_MB}MB"
  if [ "$SIZE_MB" -gt 60 ]; then
    echo "  ⚠ exceeds 60MB ceiling"
  fi
done

#!/usr/bin/env bash
set -euo pipefail

# Runs the Go backend (hot reload via air) and the Vite dev server together.
#   Backend:  127.0.0.1:7777  (API only in dev)
#   Frontend: 127.0.0.1:5173  (proxies /api -> backend)
# Open http://127.0.0.1:5173 during development.

cd "$(dirname "$0")/.."

if ! command -v air >/dev/null 2>&1; then
  echo "✗ 'air' not found. Install with:  go install github.com/air-verse/air@latest" >&2
  exit 1
fi

trap 'kill 0' EXIT INT TERM

DBVIZ_DEV=1 air -c .air.toml &
(cd web && npm run dev) &

wait

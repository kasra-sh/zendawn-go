#!/usr/bin/env bash
# Convenience wrapper — prefer ./scripts/bundle.sh linux|darwin|windows
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
exec "$ROOT/bundle.sh" windows amd64

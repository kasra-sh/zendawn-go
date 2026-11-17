#!/usr/bin/env bash
# Cross-compile a CLI bundle: binary + examples/agent.yaml + README + run helper.
# Usage: ./scripts/bundle.sh <linux|darwin|windows> [amd64|arm64]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OS="${1:-}"
ARCH="${2:-amd64}"
EXAMPLE="$ROOT/examples/agent.yaml"
CMD_PKG="$ROOT/cmd/zendawn"
BIN_NAME="zendawn"

usage() {
  echo "usage: $0 <linux|darwin|windows> [amd64|arm64]" >&2
  exit 2
}

case "$OS" in
  linux|darwin|windows) ;;
  *) usage ;;
esac
case "$ARCH" in
  amd64|arm64) ;;
  *)
    echo "unsupported arch: $ARCH (use amd64 or arm64)" >&2
    exit 2
    ;;
esac

if [[ ! -f "$EXAMPLE" ]]; then
  echo "missing example config: $EXAMPLE" >&2
  exit 1
fi
if grep -qE '^[[:space:]]*apiKey:' "$EXAMPLE"; then
  echo "refusing to bundle: $EXAMPLE still contains apiKey (use apiKeyEnv only)" >&2
  exit 1
fi
if [[ ! -d "$CMD_PKG" ]]; then
  echo "missing command package: $CMD_PKG" >&2
  exit 1
fi

OUT="$ROOT/dist/${OS}-${ARCH}"
ZIP="$ROOT/dist/zendawn-${OS}-${ARCH}.zip"
mkdir -p "$OUT"
cp "$EXAMPLE" "$OUT/agent.yaml"

EXT=""
RUN_HELPER="run.sh"
if [[ "$OS" == "windows" ]]; then
  EXT=".exe"
  RUN_HELPER="run.bat"
fi
BIN="$OUT/${BIN_NAME}${EXT}"

cat > "$OUT/README.txt" <<EOF
zendawn CLI (${OS}/${ARCH})

Requirements
  - A modern terminal for the TUI (or use: ${BIN_NAME}${EXT} -plain)

Run
  1. Open a terminal in this folder
  2. ./${RUN_HELPER}   (or on Windows: run.bat)
     or:
       ./${BIN_NAME}${EXT} -config agent.yaml -workdir .
  3. If OPENROUTER_API_KEY is not set, the app prompts for the key at startup
     (hidden input). You can also set the env var or pass -api-key.

Optional
  Linux/macOS: export ZENDAWN_CONFIG_DIR=\$HOME/.config/zendawn
  Windows:     set ZENDAWN_CONFIG_DIR=%LOCALAPPDATA%\\zendawn

Keys (TUI)
  Tab          cycle mode ask / plan / agent
  Enter        send
  Ctrl+A       stop
  Ctrl+C       quit
  Up/Down      scroll conversation
  Alt+Up/Down  prompt history
EOF

if [[ "$OS" == "windows" ]]; then
  cat > "$OUT/run.bat" <<'EOF'
@echo off
cd /d "%~dp0"
zendawn.exe -config agent.yaml -workdir %CD%
EOF
else
  cat > "$OUT/run.sh" <<EOF
#!/usr/bin/env bash
cd "\$(dirname "\$0")"
exec "./${BIN_NAME}" -config agent.yaml -workdir "\$(pwd)"
EOF
  chmod +x "$OUT/run.sh"
fi

echo "building ${BIN_NAME}${EXT} (${OS}/${ARCH})..."
(
  cd "$ROOT"
  CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -o "$BIN" ./cmd/zendawn
)

rm -f "$ZIP"
FOLDER="$(basename "$OUT")"
if command -v zip >/dev/null 2>&1; then
  (
    cd "$ROOT/dist"
    zip -r -q "$(basename "$ZIP")" "$FOLDER"
  )
else
  python3 - <<PY
import zipfile
from pathlib import Path
root = Path("$OUT")
zip_path = Path("$ZIP")
folder = "$FOLDER"
with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
    for p in sorted(root.rglob("*")):
        if p.is_file():
            zf.write(p, Path(folder) / p.relative_to(root))
print("wrote", zip_path)
PY
fi

echo "bundle: $OUT"
echo "zip:    $ZIP"
ls -la "$OUT" "$ZIP"

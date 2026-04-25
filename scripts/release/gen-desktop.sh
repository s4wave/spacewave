#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
mkdir -p "$REPO_ROOT/.tmp"
cat > "$REPO_ROOT/.tmp/spacewave.desktop" <<'DESKTOP'
[Desktop Entry]
Name=Spacewave
Comment=Self-host anything in the browser
Exec=spacewave
Icon=spacewave
Terminal=false
Type=Application
Categories=Network;P2P;
DESKTOP
echo "Generated $REPO_ROOT/.tmp/spacewave.desktop"

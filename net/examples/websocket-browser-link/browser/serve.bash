#!/bin/bash
set -eo pipefail

# Build
bash build.bash

# Copy wasm_exec.html from Go root to index.html
cp "$(go env GOROOT)/misc/wasm/wasm_exec.html" index.html

# Replace the script src path to point to local wasm_exec.js
sed -i -e 's|<script src="../../lib/wasm/wasm_exec.js"></script>|<script src="wasm_exec.js"></script>|' index.html

# Copy wasm_exec.js to current directory
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .

# Serve on port 8080
npx serve --cors


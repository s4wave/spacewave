#!/bin/bash
set -eo pipefail

echo "Building WebRTC Browser HTTP client WASM..."

# Copy the WASM exec helper from Go
cp $(go env GOROOT)/lib/wasm/wasm_exec.js ./wasm_exec.js
echo "Copied wasm_exec.js"

# Build the WASM binary
GOOS=js GOARCH=wasm go build -mod=mod -o index.wasm -v ./

echo "Build complete: index.wasm"
echo "Run ./serve.bash to start the demo server"

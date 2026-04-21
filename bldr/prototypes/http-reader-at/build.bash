#!/bin/bash
set -eo pipefail

cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .
GOOS=js GOARCH=wasm go build -o main.wasm -v ./

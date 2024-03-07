#!/bin/bash
set -eo pipefail
set -x

cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .
esbuild  --bundle --format=esm --target=es2020 --sourcemap --inject:./wasm_exec.js --outfile=main.mjs main.ts
GOOS=js GOARCH=wasm go build -o main.wasm -v ./

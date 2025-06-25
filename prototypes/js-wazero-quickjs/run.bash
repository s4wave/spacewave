#!/bin/bash
set -eo pipefail
set -x

# download quickjs wasm file
if [ ! -f qjs-wasi.wasm ]; then
    wget -O qjs-wasi.wasm "https://github.com/quickjs-ng/quickjs/releases/download/v0.10.1/qjs-wasi.wasm"
fi

esbuild --bundle --minify --tree-shaking=true --outfile=main.js main.ts
go run -v ./run.go

#!/bin/bash
set -eo pipefail
set -x

USE_TINYGO=false

if $USE_TINYGO; then
    cp $(tinygo env TINYGOROOT)/targets/wasm_exec.js ./wasm_exec.js
else
    cp $(go env GOROOT)/misc/wasm/wasm_exec.js ./wasm_exec.js
fi
esbuild \
    --bundle \
    --format=esm \
    --target=es2020 \
    --sourcemap \
    --external:util \
    --external:fs \
    --external:crypto \
    --inject:./tinygo-polyfill.js \
    --inject:./wasm_exec.js \
    --outfile=main.mjs \
    main.ts
if $USE_TINYGO; then
    if [ ! -f main.wasm ]; then
        tinygo build -o main.wasm -target wasm ./
    fi
else
    GOOS=js GOARCH=wasm go build -v -o main.wasm ./
fi
python3 -m http.server -b 127.0.0.1 8080

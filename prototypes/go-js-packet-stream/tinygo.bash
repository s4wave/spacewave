#!/bin/bash
set -eo pipefail
set -x

cp $(tinygo env TINYGOROOT)/targets/wasm_exec.js ./wasm_exec.js
esbuild  --bundle --format=esm --target=es2020 --sourcemap --external:util --external:fs --external:crypto --inject:./tinygo-polyfill.js --inject:./wasm_exec.js --outfile=main.mjs main.ts
tinygo build -o main.wasm -target wasm -no-debug -opt=2 ./

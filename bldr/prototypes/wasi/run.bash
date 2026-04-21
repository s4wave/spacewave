#!/bin/bash
set -eo pipefail

if [ ! -f ./demo.wasm ]; then
    bash build.bash
fi
wasmtime --dir=. --dir=/tmp demo.wasm

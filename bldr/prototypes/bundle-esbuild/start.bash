#!/bin/bash
set -eo pipefail

# Get the relative path to the current directory from git root
SCRIPT_DIR=$(git rev-parse --show-prefix)
SCRIPT_DIR=${SCRIPT_DIR%/}

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

npm run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/bundle-esbuild/.bldr \
    -c ./prototypes/bundle-esbuild/bldr.yaml \
    start \
    web --wasm

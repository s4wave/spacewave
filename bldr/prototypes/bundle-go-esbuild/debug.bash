#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

cd ../../cmd/bldr
dlv debug \
    --wd ../../prototypes/bundle-go-esbuild \
    -- \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/bundle-go-esbuild/.bldr \
    -c ./prototypes/bundle-go-esbuild/bldr.yaml \
    start \
    web --wasm


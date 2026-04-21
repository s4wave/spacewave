#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

npm run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/simple/.bldr \
    -c ./prototypes/simple/bldr.yaml \
    start \
    web --wasm

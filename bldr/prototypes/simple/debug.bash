#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

cd ../../cmd/bldr
dlv debug \
    --wd ../../prototypes/simple \
    -- \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/simple/.bldr \
    -c ./prototypes/simple/bldr.yaml \
    start \
    web --wasm


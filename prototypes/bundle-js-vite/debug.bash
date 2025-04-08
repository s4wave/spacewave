#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

cd ../../cmd/bldr
dlv debug \
    --wd ../../prototypes/bundle-js-vite \
    -- \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/bundle-js-vite/.bldr \
    -c ./prototypes/bundle-js-vite/bldr.yaml \
    start \
    web --wasm


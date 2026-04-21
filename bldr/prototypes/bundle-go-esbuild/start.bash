#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

npm run go:run -- \
    github.com/s4wave/spacewave/bldr/cmd/bldr \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/bundle-go-esbuild/.bldr \
    -c ./prototypes/bundle-go-esbuild/bldr.yaml \
    start \
    native

#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
    rm -rf ./.bldr
fi

npm run go:run -- \
    github.com/aperturerobotics/bldr/cmd/bldr \
    --bldr-src-path=../../../../ \
    --state-path ./prototypes/simple/.bldr \
    -c ./prototypes/simple/bldr.yaml \
    start \
    native

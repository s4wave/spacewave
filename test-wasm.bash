#!/bin/bash
set -eo pipefail

yarn clean
yarn run bldr build -b test-wasm

pushd ./.bldr/build/web/dist/dist
python3 -m http.server

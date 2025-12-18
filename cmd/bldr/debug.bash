#!/bin/bash

unset GOOS
unset GOARCH

bun run clean
dlv debug --wd ../../ -- --disable-cleanup --bldr-version=$(git rev-parse HEAD) --bldr-version-sum="" start web --wasm

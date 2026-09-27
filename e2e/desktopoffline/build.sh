#!/bin/sh
# Build the audited desktop startup closure in a private, local Bldr World.
set -eu

cd "$(dirname "$0")/../.."

export GOWORK=off GOPROXY=off GOSUMDB=off npm_config_offline=true BLDR_NO_TUI=1
go test -race -timeout=110s -count=1 ./bldr/project/starlark -run '^TestDesktopOfflineE2EConfig$'

fixture=.tmp/desktop-offline-e2e
mkdir -p "$fixture/config"
cat bldr.star desktop-offline-e2e.star > "$fixture/config/bldr.star"
cp desktop-offline-e2e.yaml "$fixture/config/bldr.yaml"

go run -mod=readonly github.com/s4wave/spacewave/bldr/cmd/bldr \
    --config="./$fixture/config/bldr.yaml" \
    --bldr-src-path=../../.. \
    --state-path="./$fixture" \
    --build-type=debug build -b desktop-offline-e2e-assets

go run -mod=readonly github.com/s4wave/spacewave/bldr/cmd/bldr \
    --config="./$fixture/config/bldr.yaml" \
    --bldr-src-path=../../.. \
    --state-path="./$fixture" \
    --build-type=debug build -b desktop-offline-e2e-dist

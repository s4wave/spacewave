#!/bin/bash
set -eo pipefail

go build -v -o forge
./forge \
    --bolt-db data.db \
    --podman-url "unix:///run/user/$(id -u)/podman/podman.sock" \
    bundle-target.yaml

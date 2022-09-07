#!/bin/bash
set -eo pipefail

go build -v -o container-volume
./container-volume \
    --bolt-db data.db \
    --podman-url "unix:///run/user/$(id -u)/podman/podman.sock" \
    fuse-test.yaml

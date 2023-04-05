#!/bin/bash
set -eo pipefail

if [ -d ./.bldr ]; then
  rm -rf .bldr
fi
go run -v ./cmd/bldr --disable-cleanup --bldr-version=$(git rev-parse HEAD) $@

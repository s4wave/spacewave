#!/bin/bash
set -eo pipefail
set -x

if [ ! -f ./main.js ]; then
  esbuild --bundle --minify --tree-shaking=true --outfile=main.js main.ts
fi
go run -v ./run.go

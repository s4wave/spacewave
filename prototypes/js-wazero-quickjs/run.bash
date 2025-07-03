#!/bin/bash
set -eo pipefail
set -x

esbuild --bundle --minify --tree-shaking=true --outfile=main.js main.ts
go run -v ./run.go

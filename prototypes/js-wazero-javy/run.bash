#!/bin/bash
set -eo pipefail
set -x

# git clone https://github.com/bytecodealliance/javy
# cd ./javy
# make cli
# cd ./target/release/
# ./javy build -o main.wasm main.js
esbuild --bundle --minify --tree-shaking=true --outfile=main.js main.ts
javy build -o main.wasm main.js
go run -v ./run.go

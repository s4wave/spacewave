#!/bin/bash
set -eo pipefail

GOOS=wasip1 GOARCH=wasm go build -v -o demo.wasm ./


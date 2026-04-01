#!/bin/bash
set -eo pipefail

echo "Testing with CGO_ENABLED=0"
CGO_ENABLED=0 go test -v ./

echo "Testing with CGO_ENABLED=1"
CGO_ENABLED=1 go test -v ./

echo "Testing with CGO_ENABLED=0 GOOS=js GOARCH=wasm"
CGO_ENABLED=0 GOOS=js GOARCH=wasm go test -v ./

echo "Tests successful."

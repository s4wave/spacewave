#!/bin/bash
set -eo pipefail

echo "Testing with CGO_ENABLED=0"
CGO_ENABLED=0 go test -v ./

echo "Testing with CGO_ENABLED=0 purego"
CGO_ENABLED=0 go test -v -tags=purego ./

echo "Testing with CGO_ENABLED=0 modernc"
CGO_ENABLED=0 go test -v -tags=purego,sqlite_purego_modernc ./

echo "Testing with CGO_ENABLED=1"
CGO_ENABLED=1 go test -v ./

# TODO: uncomment once implemented
# echo "Testing with CGO_ENABLED=0 GOOS=js GOARCH=wasm"
# CGO_ENABLED=0 GOOS=js GOARCH=wasm go test -v ./

echo "Tests successful."

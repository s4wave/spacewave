#!/bin/bash

cp $(go env GOROOT)/lib/wasm/wasm_exec.js ./wasm_exec.js
GOOS=js GOARCH=wasm go build -o test.wasm -v ./

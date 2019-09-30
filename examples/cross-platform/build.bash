#!/bin/bash

GOOS=js GOARCH=wasm go build -o example.wasm -v ./

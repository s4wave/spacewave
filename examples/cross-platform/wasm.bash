#!/bin/bash
set -eo pipefail

./build.bash
cd ./proxy
echo "Browse to http://localhost:5000"
go run -v ./

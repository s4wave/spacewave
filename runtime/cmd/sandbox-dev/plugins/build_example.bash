#!/bin/bash
set -eo pipefail

export GO111MODULE=on
export $(go env | grep GOOS)
GOOS="${GOOS%\"}"
GOOS="${GOOS#\"}"
if [[ "$GOOS" != "linux" ]]; then
    echo "This only works on GOOS=linux."
    exit 1
fi

echo "Building plugin binary..."

export CONTROLLER_BUS_CODEGEN_DIR="" # use tmpdir
export CONTROLLER_BUS_PLUGIN_BINARY_ID="controllerbus/examples/hot-demo/codegen-demo/1"
export CONTROLLER_BUS_OUTPUT="$(pwd)/example-badger.cbus.so"
export CONTROLLER_BUS_PLUGIN_BUILD_PREFIX="cbus-demo"
# export CONTROLLER_BUS_NO_CLEANUP="true"

go run -v \
   -trimpath \
   github.com/aperturerobotics/controllerbus/cmd/controllerbus -- \
   hot compile \
   "github.com/aperturerobotics/hydra/volume/badger"

echo "Compiled successfully."


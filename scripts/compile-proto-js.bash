#!/bin/bash

go mod vendor
SRC_PATH=$(pwd)/vendor
pbjs \
    -t static-module \
    -w commonjs  \
    -o src/proto/proto.js \
    -p . \
    -p ${SRC_PATH} \
    ${SRC_PATH}/github.com/aperturerobotics/osbundle/osbundle.proto \
    ./runtime/osflash/osflash.proto \
    ./runtime/shellquery/shellquery.proto \
    ./runtime/pstream/pstream.proto \
    ./runtime/ipc/ipc.proto
pbts -o src/proto/proto.d.ts src/proto/proto.js

#!/bin/bash

unset GOOS
unset GOARCH

yarn clean
dlv debug --wd ../../ -- --disable-cleanup --bldr-version=$(git rev-parse HEAD) --bldr-version-sum="" start web

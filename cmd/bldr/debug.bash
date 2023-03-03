#!/bin/bash

rm -rf ../../.bldr
dlv debug --wd ../../ -- --disable-cleanup --bldr-version=$(git rev-parse HEAD) --bldr-version-sum="" start web

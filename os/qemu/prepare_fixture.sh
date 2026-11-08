#!/bin/sh
# Run inside the development tool image with existing inputs and an absent fixture path.
set -eu
inputs=$1
fixture=$2
if [ -e "$fixture" ]; then
    echo "Fixture already exists: $fixture" >&2
    exit 1
fi
mkdir "$fixture"
mkdir "$fixture/system" "$fixture/configuration"
GOMAXPROCS=2 "$inputs/init" seed "$fixture/system/image" /bin/busybox "$inputs/apps.tar"
GOMAXPROCS=2 "$inputs/init" seed-data "$fixture/system/data"
GOMAXPROCS=2 "$inputs/init" seed-data "$fixture/configuration/data"
truncate -s 2G "$fixture/system.img"
mkfs.btrfs -f --rootdir "$fixture/system" "$fixture/system.img"
truncate -s 2G "$fixture/configuration.img"
mkfs.btrfs -f --rootdir "$fixture/configuration" "$fixture/configuration.img"

#!/bin/sh
# install.sh writes an installable Spacewave OS disk: a Btrfs system store
# holding the signed image and its card, at its content's size, beside the boot
# set that boots it. First boot grows the store to its disk, pins the card's
# signers, verifies the authorization, and only then creates the data Volume,
# so the disk carries no data Volume and no key.
# Usage: install.sh IMAGES IMAGE INIT CARD AUTHORIZATION OUT
#   IMAGES holds bzImage and rootfs.cpio.lz4; IMAGE is the image store; INIT is
#   the static spacewave-init; CARD holds the Space's signers and grant;
#   AUTHORIZATION holds the owner's authorization of the image root; OUT
#   receives disk.img, bzImage and rootfs.cpio.lz4.
set -eu

if [ "$#" -ne 6 ]; then
    echo "usage: install.sh IMAGES IMAGE INIT CARD AUTHORIZATION OUT" >&2
    exit 2
fi
images=$1
image=$2
init=$3
card=$4
authorization=$5
out=$6

# Copy the image store into a fresh system root; the init refuses a card that
# does not authorize this store's selected root.
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
install -m 755 "$init" "$fixture/init"
mkdir -m 700 "$fixture/root"
cp -R "$image" "$fixture/root/image"
chmod -R u+w "$fixture/root/image"
GOMAXPROCS=2 "$fixture/init" install-card "$fixture/root/image" \
    "$card/signers" "$authorization/authorization" "$card/grant"

# Write the store at its content's size; first boot grows it to the disk.
mkdir -p "$out"
truncate -s 1G "$out/disk.img"
mkfs.btrfs -q --shrink --rootdir "$fixture/root" "$out/disk.img"
cp "$images/bzImage" "$images/rootfs.cpio.lz4" "$out/"

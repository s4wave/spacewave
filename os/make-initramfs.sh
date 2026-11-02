#!/bin/sh
# Package only the static executable and an optional matching FUSE module.
set -eu
if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: $0 STATIC_INIT OUTPUT.cpio.gz [UNCOMPRESSED_FUSE.ko]" >&2
	exit 2
fi
if [ "$(id -u)" != 0 ]; then
	echo "run as root to package /dev/console for the kernel's initial stdio" >&2
	exit 2
fi
init=$(realpath "$1")
output=$(realpath -m "$2")
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir "$stage/root"
cp "$init" "$stage/root/init"
chmod 755 "$stage/root/init"
mkdir "$stage/root/proc" "$stage/root/dev" "$stage/root/tmp"
mknod "$stage/root/dev/console" c 5 1
chmod 600 "$stage/root/dev/console"
if [ "$#" = 3 ]; then
	cp "$3" "$stage/root/fuse.ko"
fi
if command -v cpio >/dev/null 2>&1; then
	(cd "$stage/root" && find . -print0 | cpio --null --create --format=newc --owner=0:0 > "$stage/archive.cpio")
else
	bsdtar --format=newc --uid 0 --gid 0 -cf "$stage/archive.cpio" -C "$stage/root" .
fi
gzip -1 -c "$stage/archive.cpio" > "$output"
echo "$output"

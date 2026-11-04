#!/bin/sh
# Build Linux's pinned initramfs generator in a fresh private host prefix.
set -eu
if [ "$#" != 1 ]; then
	echo "usage: $0 FRESH_PREFIX" >&2
	exit 2
fi
prefix=$1
mkdir "$prefix"
prefix=$(cd "$prefix" && pwd -P)
source_url=https://raw.githubusercontent.com/torvalds/linux/v6.17/usr/gen_init_cpio.c
source_sha256=309cab96d2b43133a0758f165fbcf9cc1b671623555ef8dadd9983095c462345
curl --fail --location --silent --show-error --connect-timeout 15 --max-time 120 \
	"$source_url" -o "$prefix/gen_init_cpio.c"
printf '%s  %s\n' "$source_sha256" "$prefix/gen_init_cpio.c" | sha256sum -c -
"${CC:-cc}" -O2 -o "$prefix/gen_init_cpio" "$prefix/gen_init_cpio.c"
echo "$prefix/gen_init_cpio"

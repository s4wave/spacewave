#!/bin/bash
# Replace the board's initramfs with one whose only program is spacewave-init,
# so the boot set leaves out the Buildroot root filesystem. Board scripts load
# rootfs.cpio.lz4 by name, so list spacewave/os last in SKIFF_CONFIG.
#
# SPACEWAVE_OS_BOOT names a directory holding "init", the static spacewave-init
# executable built for the board's architecture, and "make-initramfs.sh" from
# the os directory. The package reads nothing outside itself and that
# directory, so a build that receives both as inputs is keyed by their bytes.
set -euo pipefail

# Refuse a build without the boot executable.
boot=${SPACEWAVE_OS_BOOT:-}
if [ -z "$boot" ] || [ ! -f "$boot/init" ] || [ ! -f "$boot/make-initramfs.sh" ]; then
	echo "spacewave/os: set SPACEWAVE_OS_BOOT to a directory holding init and make-initramfs.sh" >&2
	exit 1
fi

# Use the kernel tree's own archive tool, which every initrd kernel builds.
tools=("$BUILD_DIR"/linux-*/usr/gen_init_cpio)
if [ "${#tools[@]}" -ne 1 ] || [ ! -x "${tools[0]}" ]; then
	echo "spacewave/os: expected one built gen_init_cpio, found: ${tools[*]}" >&2
	exit 1
fi

# Build beside the images, then rename over the board's initramfs.
staged=$BINARIES_DIR/.spacewave-os.cpio.lz4
rm -f "$staged"
PATH=$HOST_DIR/bin:$PATH GEN_INIT_CPIO=${tools[0]} \
	sh "$boot/make-initramfs.sh" "$boot/init" "$staged"
mv -f "$staged" "$BINARIES_DIR/rootfs.cpio.lz4"

# Remove every other root image, including ones an earlier build of the
# workspace left behind, since Buildroot never clears the images directory.
for image in "$BINARIES_DIR"/rootfs.*; do
	if [ "$image" != "$BINARIES_DIR/rootfs.cpio.lz4" ]; then
		rm -rf "$image"
	fi
done
echo "spacewave/os: rootfs.cpio.lz4 holds spacewave-init alone"

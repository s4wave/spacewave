#!/bin/sh
# Package one static userspace ELF and dependency-ordered matching kernel modules.
set -eu
if [ "$#" -lt 2 ] || [ "$#" -gt 18 ]; then
	echo "usage: $0 STATIC_INIT OUTPUT.cpio.gz [UNCOMPRESSED_MODULE.ko ...]" >&2
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
shift 2
mkdir "$stage/root/modules"
i=0
for module do
	case "$module" in
		*.ko) ;;
		*) echo "expected an uncompressed .ko: $module" >&2; exit 2 ;;
	esac
	cp "$module" "$stage/root/modules/$(printf '%02d' "$i")-$(basename "$module")"
	i=$((i + 1))
done
if command -v cpio >/dev/null 2>&1; then
	(cd "$stage/root" && find . -print0 | cpio --null --create --format=newc --owner=0:0 > "$stage/archive.cpio")
else
	bsdtar --format=newc --uid 0 --gid 0 -cf "$stage/archive.cpio" -C "$stage/root" .
fi
gzip -1 -c "$stage/archive.cpio" > "$output"
echo "$output"

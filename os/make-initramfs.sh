#!/bin/sh
# Package static init and ordered matching modules without root or host devices.
set -eu
if [ "$#" -lt 2 ] || [ "$#" -gt 18 ]; then
	echo "usage: $0 STATIC_INIT OUTPUT.cpio.{gz,lz4} [UNCOMPRESSED_MODULE.ko ...]" >&2
	exit 2
fi
# The output suffix picks the compressor; lz4 uses the legacy frame Linux reads.
case "$2" in
	*.cpio.gz) compress="gzip -n -1" ;;
	*.cpio.lz4) compress="lz4 -l -9 -q -c" ;;
	*) echo "output must end in .cpio.gz or .cpio.lz4: $2" >&2; exit 2 ;;
esac
generator=$(command -v "${GEN_INIT_CPIO:-gen_init_cpio}") || {
	echo "set GEN_INIT_CPIO to the host tool built by os/qemu/build-initramfs-tool.sh" >&2
	exit 2
}
generator=$(realpath "$generator")
init=$(realpath "$1")
output_dir=$(cd "$(dirname "$2")" && pwd -P)
output="$output_dir/$(basename "$2")"
if [ -e "$output" ] || [ -L "$output" ]; then
	echo "output must be fresh: $output" >&2
	exit 2
fi
stage=$(mktemp -d "$output_dir/.initramfs.XXXXXX")
generator_pid=
cleanup() {
	if [ -n "$generator_pid" ]; then
		kill "$generator_pid" 2>/dev/null || :
		wait "$generator_pid" 2>/dev/null || :
	fi
	rm -rf "$stage"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

# Fixed staging names keep source paths with spaces out of the kernel text spec.
cp "$init" "$stage/init"
cat > "$stage/spec" <<'EOF'
dir /proc 0755 0 0
dir /dev 0755 0 0
dir /tmp 0755 0 0
dir /modules 0755 0 0
nod /dev/console 0600 0 0 c 5 1
file /init init 0755 0 0
EOF
shift 2
i=0
for module do
	case "$module" in
		*.ko) ;;
		*) echo "expected an uncompressed .ko: $module" >&2; exit 2 ;;
	esac
	name=$(basename "$module")
	case "$name" in
		*[!a-zA-Z0-9_.+-]*) echo "unsupported module filename: $name" >&2; exit 2 ;;
	esac
	number=$(printf '%02d' "$i")
	cp "$module" "$stage/module-$number"
	printf 'file /modules/%s-%s module-%s 0644 0 0\n' "$number" "$name" "$number" >> "$stage/spec"
	i=$((i + 1))
done

# Stream through a FIFO so POSIX sh can check both processes, not just the
# compressor. The kernel tool uses a 64 KiB file buffer; -t 0 fixes entry mtimes.
mkfifo "$stage/archive.cpio"
(cd "$stage" && exec "$generator" -t 0 spec) > "$stage/archive.cpio" &
generator_pid=$!
packed=0
$compress < "$stage/archive.cpio" > "$stage/archive.cpio.packed" || packed=$?
generated=0
wait "$generator_pid" || generated=$?
generator_pid=
if [ "$generated" -ne 0 ] || [ "$packed" -ne 0 ]; then
	echo "initramfs failed: gen_init_cpio=$generated compress=$packed" >&2
	exit 1
fi

# A same-filesystem hard link publishes only complete bytes and cannot clobber.
ln "$stage/archive.cpio.packed" "$output"
echo "$output"

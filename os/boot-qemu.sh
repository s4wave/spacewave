#!/bin/sh
# Boot with optional retained Btrfs backing, without network or host directories.
set -eu
if [ "$#" != 3 ] && [ "$#" != 5 ]; then
	echo "usage: $0 VMLINUZ INITRAMFS SERIAL.log [BTRFS_IMAGE SEED.log]" >&2
	exit 2
fi
kernel=$(realpath "$1")
image=$(realpath "$2")
log=$(realpath -m "$3")
append='console=ttyS0 rdinit=/init panic=-1 quiet GOMAXPROCS=1'
seed=
if [ "$#" = 5 ]; then
	disk=$(realpath "$4")
	seed=$(realpath "$5")
	test -f "$disk"
	append="$append SPACEWAVE_OS_STORE=/store/image"
	set -- -drive "file=$disk,format=raw,if=virtio,cache=none"
else
	set --
fi
accel=${ACCEL:-kvm}
cpu=host
if [ "$accel" = tcg ]; then cpu=max; fi
timeout 90 qemu-system-x86_64 -accel "$accel" -cpu "$cpu" -m 768 -smp 1 \
	-nodefaults -no-reboot -display none -serial "file:$log" \
	-kernel "$kernel" -initrd "$image" \
	-append "$append" "$@"
for marker in PID1 FROZEN MMAP_OK READONLY_OK WORKLOAD_OK EXEC_OK UNMOUNT_OK PASS POWEROFF; do
	grep -F "SPACEWAVE_OS_$marker" "$log"
done
grep -F 'SPACEWAVE_OS_PID1 gomaxprocs=1' "$log"
if grep -E 'SPACEWAVE_OS_(FAILED|ERROR)|Kernel panic' "$log"; then
	exit 1
fi
grep -F 'Power down' "$log"
if [ -n "$seed" ]; then
	for marker in BACKING_MOUNTED REOPEN CONTENT BACKING_UNMOUNT_OK; do
		grep -F "SPACEWAVE_OS_$marker" "$log"
	done
	expected=$(grep -E 'SPACEWAVE_OS_(REOPEN|CONTENT)' "$seed" | tr -d '\r')
	actual=$(grep -E 'SPACEWAVE_OS_(REOPEN|CONTENT)' "$log" | tr -d '\r')
	test "$expected" = "$actual"
fi

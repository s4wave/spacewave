#!/bin/sh
# Boot without any guest disk, network, graphical display or host directory.
set -eu
if [ "$#" != 3 ]; then
	echo "usage: $0 VMLINUZ INITRAMFS SERIAL.log" >&2
	exit 2
fi
kernel=$(realpath "$1")
image=$(realpath "$2")
log=$(realpath -m "$3")
timeout 90 qemu-system-x86_64 -accel "${ACCEL:-kvm}" -m 768 -smp 1 +	-nodefaults -no-reboot -display none -serial "file:$log" +	-kernel "$kernel" -initrd "$image" +	-append 'console=ttyS0 rdinit=/init panic=-1 quiet'
for marker in PID1 FROZEN MMAP_OK READONLY_OK WORKLOAD_OK EXEC_OK UNMOUNT_OK PASS POWEROFF; do
	grep -F "SPACEWAVE_OS_$marker" "$log"
done
if grep -E 'SPACEWAVE_OS_(FAILED|ERROR)|Kernel panic' "$log"; then
	exit 1
fi
grep -F 'Power down' "$log"

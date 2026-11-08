#!/bin/sh
# Run existing disposable fixtures only. QEMU owns terminal setup and restoration.
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: sh os/qemu/run-console.sh KERNEL INITRAMFS BTRFS-DISK" >&2
    exit 2
fi
if [ ! -t 0 ] || [ ! -t 1 ]; then
    echo "run-console: attach a terminal (or PTY); Ctrl-a x must remain reachable" >&2
    exit 2
fi
kernel=$1
initramfs=$2
disk=$3
for input in "$kernel" "$initramfs" "$disk"; do
    if [ ! -f "$input" ] || [ ! -r "$input" ]; then
        echo "run-console: expected an existing readable regular file: $input" >&2
        exit 2
    fi
done
case "$disk" in
    *,*) echo "run-console: the disk path must not contain a comma" >&2; exit 2 ;;
esac
if [ ! -w "$disk" ]; then
    echo "run-console: the disposable disk must be writable: $disk" >&2
    exit 2
fi
accel=${ACCEL:-kvm}
case "$accel" in
    kvm|tcg) ;;
    *) echo "run-console: ACCEL must be kvm or tcg" >&2; exit 2 ;;
esac
cpu=host
if [ "$accel" = tcg ]; then cpu=max; fi
native=${NATIVE_BENCH:-0}
case "$native" in
    0|1) ;;
    *) echo "run-console: NATIVE_BENCH must be 0 or 1" >&2; exit 2 ;;
esac

echo "Console: Ctrl-C goes to the guest. Ctrl-a x exits QEMU, even if startup fails."
echo "Graceful shutdown: finish jobs, then run poweroff (or sync; exit)."
# signal=off sends Ctrl-C as serial input instead of killing the host emulator.
# The standard mux keeps Ctrl-a x and Ctrl-a c available throughout startup.
exec "${QEMU:-qemu-system-x86_64}" \
    -no-user-config -nodefaults -machine "accel=$accel" -cpu "$cpu" -smp 1 -m 768M \
    -display none -nic none -no-reboot \
    -chardev stdio,id=console,mux=on,signal=off \
    -serial chardev:console -object monitor-hmp,id=monitor,chardev=console,readline=on \
    -kernel "$kernel" -initrd "$initramfs" \
    -append "console=ttyS0,115200 rdinit=/init panic=-1 quiet GOMAXPROCS=1 SPACEWAVE_OS_MODE=console SPACEWAVE_OS_STORE=/store/image SPACEWAVE_OS_DATA=/store/data SPACEWAVE_OS_TTY=/dev/ttyS0 SPACEWAVE_OS_NATIVE_BENCH=$native" \
    -drive "file=$disk,format=raw,if=virtio,cache=none"

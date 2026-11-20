#!/bin/sh
# rehearse.sh boots a boot set with a freshly seeded system root in QEMU. It
# succeeds only when the console guest reports ready, writes and syncs a file
# in its data Volume, and closes every store on poweroff, and when the kill
# trial then keeps every acknowledged record across two SIGKILLs. Run it in
# the os/qemu tool image with /dev/kvm, as the rehearsal Step does.
# Usage: rehearse.sh IMAGES INIT RECEIPT
#   IMAGES holds bzImage and rootfs.cpio.lz4; INIT is the static spacewave-init
#   that seeds the root; RECEIPT is written once the guest stops cleanly.
# The guest's console streams to stdout, so a failed boot keeps its log.
set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: rehearse.sh IMAGES INIT RECEIPT" >&2
    exit 2
fi
images=$1
init=$2
receipt=$3
tools=$(cd "$(dirname "$0")" && pwd)

# Seed the frozen image and the data World into a fresh 4 GiB disk, which
# clears the data Volume's disk reserve.
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
install -m 755 "$init" "$fixture/init"
init=$fixture/init
mkdir -m 700 "$fixture/root"
GOMAXPROCS=2 "$init" seed "$fixture/root/image" /bin/busybox
GOMAXPROCS=2 "$init" seed-data "$fixture/root/data"
truncate -s 4G "$fixture/btrfs.img"
mkfs.btrfs -q --rootdir "$fixture/root" "$fixture/btrfs.img"

# Once the console is ready, write one file through the data Volume and power
# off; the guest prints SPACEWAVE_OS_PASS only after every store closed. The
# runner turns echo off before it types these lines, so the marker appears only
# as output, after the shell's prompt.
cat > "$fixture/startup.sh" <<'EOF'
mkdir /data/rehearsal && echo rehearsed > /data/rehearsal/probe && sync /data/rehearsal/probe && echo SPACEWAVE_OS_REHEARSAL_WROTE
poweroff
EOF

# Bound the boot, so a guest that hangs fails the Step instead of holding it.
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/run_app.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/btrfs.img" \
    --startup "$fixture/startup.sh" --receipt "$fixture/stopped" | tee "$fixture/console.log"
if ! grep -q 'SPACEWAVE_OS_REHEARSAL_WROTE' "$fixture/console.log" || [ ! -f "$fixture/stopped" ]; then
    echo "rehearsal failed: the guest did not write its data Volume and stop cleanly" >&2
    exit 1
fi

# Kill the guest twice mid-write on the same disk; every record it
# acknowledged must survive each reopen.
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/kill_trial.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/btrfs.img"
mv "$fixture/stopped" "$receipt"

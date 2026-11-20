#!/bin/sh
# rehearse.sh boots a boot set on a system root holding an image store in
# QEMU. It succeeds only when the console guest grows its store to the disk,
# creates its data Volume, reports ready, writes and syncs a file in that
# Volume, and closes every store on poweroff, and when the kill trial then
# keeps every acknowledged record across two SIGKILLs. Run it in the os/qemu
# tool image with /dev/kvm, as the rehearsal Step does.
# Usage: rehearse.sh IMAGES IMAGE INIT RECEIPT
#   IMAGES holds bzImage and rootfs.cpio.lz4; IMAGE is the image store the
#   install Step also writes; INIT is the static spacewave-init that writes a
#   rehearsal card; RECEIPT is written once the guest stops cleanly.
# The guest's console streams to stdout, so a failed boot keeps its log.
set -eu

if [ "$#" -ne 4 ]; then
    echo "usage: rehearse.sh IMAGES IMAGE INIT RECEIPT" >&2
    exit 2
fi
images=$1
image=$2
init=$3
receipt=$4
tools=$(cd "$(dirname "$0")" && pwd)

# Authorize a copy of the image with a throwaway signer and leave the data
# Volume absent, so the boot pins, verifies and creates it as an install does.
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
install -m 755 "$init" "$fixture/init"
mkdir -m 700 "$fixture/root"
cp -R "$image" "$fixture/root/image"
chmod -R u+w "$fixture/root/image"
GOMAXPROCS=2 "$fixture/init" rehearsal-card "$fixture/root/image"

# Write the store at its content's size on a 4 GiB disk, which clears the data
# Volume's disk reserve once first boot grows the store.
truncate -s 1G "$fixture/btrfs.img"
mkfs.btrfs -q --shrink --rootdir "$fixture/root" "$fixture/btrfs.img"
truncate -s 4G "$fixture/btrfs.img"

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
for marker in SPACEWAVE_OS_BACKING_GROWN SPACEWAVE_OS_DATA_CREATED SPACEWAVE_OS_REHEARSAL_WROTE; do
    if ! grep -q "$marker" "$fixture/console.log"; then
        echo "rehearsal failed: the guest never printed $marker" >&2
        exit 1
    fi
done
if [ ! -f "$fixture/stopped" ]; then
    echo "rehearsal failed: the guest did not stop cleanly" >&2
    exit 1
fi

# Kill the guest twice mid-write on the same disk; every record it
# acknowledged must survive each reopen.
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/kill_trial.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/btrfs.img"
mv "$fixture/stopped" "$receipt"

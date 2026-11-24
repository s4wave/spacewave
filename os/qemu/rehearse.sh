#!/bin/sh
# rehearse.sh boots a boot set on a system root holding an image store in
# QEMU. It succeeds only when the console guest grows its store to the disk,
# creates its data Volume, reports ready, writes and syncs a file in that
# Volume, and closes every store on poweroff; when the kill trial then keeps
# every acknowledged record across two SIGKILLs; when the revision trial moves
# the guest to a staged revision across power cuts in staging and in the trial
# boot, and rolls a broken one back, with that file intact; and when a copy whose card an unpinned signer authorized refuses to
# boot. Run it in the os/qemu tool image
# with /dev/kvm, as the rehearsal Step does.
# Usage: rehearse.sh IMAGES IMAGE INIT RECEIPT
#   IMAGES holds bzImage and rootfs.cpio.lz4; IMAGE is the image store the
#   install Step also writes; INIT is the static spacewave-init that writes a
#   rehearsal card; RECEIPT is a directory written only after every check
#   passes, whose root file names the image root the guest authorized and
#   booted, so a signer can refuse a receipt for another image.
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

# Copy the image and leave the data Volume absent, so the boot pins, verifies
# and creates it as an install does.
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
install -m 755 "$init" "$fixture/init"
mkdir -m 700 "$fixture/root"
cp -R "$image" "$fixture/root/image"
chmod -R u+w "$fixture/root/image"

# Seed two revisions for the guest to stage under the image's transform, as a
# Space builds each revision under its own: good, which differs from the image
# by one file, and broken, whose workload is an init one byte longer than the
# boot set's, so its image check fails. Seeding reads the built image, which
# the card's authorization below makes the accepted revision.
mkdir -p "$fixture/revision/etc" "$fixture/root/staging/good" "$fixture/root/staging/broken"
echo good > "$fixture/revision/etc/spacewave-revision"
tar -C "$fixture/revision" -cf "$fixture/revision.tar" etc/spacewave-revision
GOMAXPROCS=2 "$fixture/init" seed-revision "$fixture/root/image" "$fixture/root/staging/good/image" /bin/busybox "$fixture/revision.tar"
cp "$fixture/init" "$fixture/broken-init"
printf x >> "$fixture/broken-init"
GOMAXPROCS=2 "$fixture/broken-init" seed-revision "$fixture/root/image" "$fixture/root/staging/broken/image" /bin/busybox "$fixture/revision.tar"

# Authorize the image with a throwaway signer, and sign both revisions with
# the same key.
GOMAXPROCS=2 "$fixture/init" rehearsal-card "$fixture/root/image" "$fixture/key"
for revision in good broken; do
    GOMAXPROCS=2 "$fixture/init" rehearsal-sign "$fixture/root/staging/$revision/image" "$fixture/key" \
        "$fixture/root/staging/$revision/authorization"
done

# Write the store at its content's size on a 4 GiB disk, which clears the data
# Volume's disk reserve once first boot grows the store.
truncate -s 1G "$fixture/btrfs.img"
mkfs.btrfs -q --shrink --rootdir "$fixture/root" "$fixture/btrfs.img"
truncate -s 4G "$fixture/btrfs.img"

# Once the console is ready, write one file through the data Volume and power
# off; the guest prints SPACEWAVE_OS_PASS only after every store closed. The
# runner turns echo off before it types these lines, so the marker appears only
# as output, after the shell's prompt. An image whose resolv.conf holds the
# guest's DHCP lease also prints SPACEWAVE_OS_RESOLVER_BOUND, and an image
# holding the daemon prints SPACEWAVE_OS_DAEMON_LISTENING once its socket
# appears, within a minute.
cat > "$fixture/startup.sh" <<'EOF'
{ while read -r key value; do [ "$key" = nameserver ] && echo SPACEWAVE_OS_RESOLVER_BOUND && break; done < /etc/resolv.conf; } 2>/dev/null
i=0; [ -x /usr/bin/spacewave ] && while [ $i -lt 60 ]; do [ -S /var/lib/spacewave/spacewave.sock ] && echo SPACEWAVE_OS_DAEMON_LISTENING && break; sleep 1; i=$((i+1)); done
mkdir /data/rehearsal && echo rehearsed > /data/rehearsal/probe && sync /data/rehearsal/probe && echo SPACEWAVE_OS_REHEARSAL_WROTE
poweroff
EOF

# Bound the boot, so a guest that hangs fails the Step instead of holding it.
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/run_app.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/btrfs.img" \
    --startup "$fixture/startup.sh" --receipt "$fixture/stopped" --dhcp | tee "$fixture/console.log"
for marker in SPACEWAVE_OS_BACKING_GROWN SPACEWAVE_OS_AUTHORIZED SPACEWAVE_OS_DATA_CREATED SPACEWAVE_OS_REHEARSAL_WROTE; do
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

# Stage good across a power cut, survive a cut trial boot by rolling back and
# restaging, accept good, then stage broken and boot back to good; the data
# Volume keeps the probe the first boot wrote.
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/revision_trial.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/btrfs.img"

# Pin the first card's signer on a fresh copy whose card a second throwaway
# key signed, as a card rewritten after first boot would be; the boot must
# refuse the image before opening it or its data.
mkdir -m 700 "$fixture/refused" "$fixture/refused/data"
cp -R "$image" "$fixture/refused/image"
chmod -R u+w "$fixture/refused/image"
GOMAXPROCS=2 "$fixture/init" rehearsal-card "$fixture/refused/image" "$fixture/refused-key"
install -m 600 "$fixture/root/image/signers" "$fixture/refused/data/pinned-signers"
truncate -s 1G "$fixture/refused.img"
mkfs.btrfs -q --shrink --rootdir "$fixture/refused" "$fixture/refused.img"
truncate -s 4G "$fixture/refused.img"
timeout -k 30 "${REHEARSAL_TIMEOUT:-600}" python3 "$tools/refusal_trial.py" \
    --accel "${ACCEL:-kvm}" --kernel "$images/bzImage" \
    --initramfs "$images/rootfs.cpio.lz4" --disk "$fixture/refused.img"

# Record the root the guest authorized and booted; the receipt appears last.
booted=$(tr -d '\r' < "$fixture/console.log" | sed -n 's/.*SPACEWAVE_OS_AUTHORIZED signer=[^ ]* root=\([^ ]*\).*/\1/p' | head -n 1)
if [ -z "$booted" ]; then
    echo "rehearsal failed: the guest never named its authorized root" >&2
    exit 1
fi
mkdir -p "$fixture/receipt"
printf '%s\n' "$booted" > "$fixture/receipt/root"
mv "$fixture/receipt" "$receipt"

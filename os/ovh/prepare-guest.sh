#!/bin/sh
# Prepare only the explicitly selected disposable Debian guest, preserving recovery boot.
set -eu
expected_uuid=${1:?usage: prepare-guest.sh EXPECTED_INSTANCE_UUID}
test "$(id -u)" = 0
test "$(cat /sys/class/dmi/id/product_uuid)" = "$expected_uuid"

# Prevent package hooks from starting services; never replace an existing policy.
policy=/usr/sbin/policy-rc.d
created_policy=false
if test ! -e "$policy"; then
    printf '#!/bin/sh\nexit 101\n' > "$policy"
    chmod 755 "$policy"
    created_policy=true
fi
cleanup() {
    if "$created_policy"; then rm -f "$policy"; fi
}
trap cleanup EXIT HUP INT TERM
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y --no-install-recommends --no-upgrade \
    qemu-system-x86 qemu-utils busybox-static cpio initramfs-tools-core \
    btrfs-progs e2fsprogs fuse3 python3 tmux socat jq

# Keep all experimental images, mounts and artifacts outside the recovery boot tree.
install -d -o debian -g debian /srv/spacewave-os \
    /srv/spacewave-os/boot /srv/spacewave-os/storage \
    /srv/spacewave-os/results /srv/spacewave-os/src
usermod -aG kvm debian
modprobe fuse
modprobe loop
modprobe btrfs
test -c /dev/fuse
test -c /dev/kvm
qemu-system-x86_64 --version | head -1
ls -l /dev/fuse /dev/kvm

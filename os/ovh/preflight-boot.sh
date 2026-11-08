#!/bin/sh
# Boot only a new nested initramfs; attach neither guest nor host disks.
set -eu
root=$(mktemp -d /srv/spacewave-os/boot/preflight.XXXXXX)
mkdir -p "$root/tree/bin" "$root/tree/proc" "$root/tree/sys" "$root/tree/dev"
cp /usr/bin/busybox "$root/tree/bin/busybox"
ln -s busybox "$root/tree/bin/sh"
cat > "$root/tree/init" <<'INIT'
#!/bin/sh
/bin/busybox mount -t proc proc /proc
/bin/busybox mount -t sysfs sys /sys
/bin/busybox mount -t devtmpfs dev /dev
echo SPACEWAVE_NESTED_KVM_READY
/bin/busybox uname -a
/bin/busybox poweroff -f
INIT
chmod 755 "$root/tree/init"
cp "/boot/vmlinuz-$(uname -r)" "$root/vmlinuz"
(cd "$root/tree" && find . -print0 | cpio --null -o --format=newc 2>/dev/null | gzip -1) > "$root/initramfs.cpio.gz"
timeout 45s qemu-system-x86_64 -enable-kvm -cpu host -smp 1 -m 512 \
    -kernel "$root/vmlinuz" -initrd "$root/initramfs.cpio.gz" \
    -append 'console=ttyS0 rdinit=/init panic=-1' \
    -display none -serial stdio -monitor none -nic none -no-reboot \
    > "$root/serial.log" 2>&1
grep SPACEWAVE_NESTED_KVM_READY "$root/serial.log"
printf 'Nested boot artifacts: %s\n' "$root"

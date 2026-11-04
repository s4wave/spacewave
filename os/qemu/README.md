# Unprivileged QEMU tools

`install-tools.sh` extracts a pinned Debian amd64 bundle for forge's Debian
forky/sid host (glibc 2.43). It uses installed curl, dpkg-deb, readelf and ldd,
without sudo, apt installation, mounts or services. The helper records every
package version, SHA256 and official download URL. Run from the source checkout:

```sh
sh -n os/qemu/install-tools.sh
sh os/qemu/install-tools.sh /home/core/build-artifacts/spacewave-os-qemu-tools/bundle
```

The prefix must be fresh, with an existing parent directory. An interrupted
installation leaves its files for inspection; choose a fresh prefix to retry.
The Debian mirror can retire versions; retain `downloads/` for later offline
re-extraction with `dpkg-deb -x`. Never substitute a different package under a
pinned hash.

The bundle contains QEMU 11.1.3, btrfs-progs 7.3, GNU cpio 2.15 and static
BusyBox 1.38.0. Wrappers set library lookup for one process and QEMU's firmware
directory explicitly. They work after logout or reboot without shell setup.
The helper adds relative SeaBIOS aliases in that private firmware directory.
For example:

```sh
tools=/home/core/build-artifacts/spacewave-os-qemu-tools/bundle
"$tools/bin/qemu-system-x86_64" --version
"$tools/bin/cpio" --version
"$tools/bin/mkfs.btrfs" --version
readelf -lW "$tools/root/usr/bin/busybox"  # must have neither INTERP nor DYNAMIC
"$tools/bin/busybox" sh -c 'echo static-userspace-ok'
```

Use `root/usr/bin/busybox`, the actual static ELF, when packaging guest content;
`bin/busybox` is a host shell wrapper. To select these tools for the existing
boot helper, change PATH only for that command:

```sh
PATH="$tools/bin:$PATH" ACCEL=tcg sh os/boot-qemu.sh \
  /absolute/vmlinuz /absolute/initramfs.cpio.gz /absolute/new-serial.log
```

Forge lacks user KVM access. Real guest boot remains a separate check. The bundle
supports headless serial, raw disks and TCG; it deliberately omits optional GUI,
USB, network boot firmware and privileged bridge/helper programs. Extraction does
not run maintainer scripts, register packages, activate services or resolve apt
dependencies. It fills the selected binaries' missing runtime libraries, including
their transitive dependencies, and uses the host's remaining libraries. It is
reusable on this host, not a hermetic Linux distribution. Installer ldd reports
record the actual library paths; a different host may need other libraries.

## Fresh offline Btrfs image

This check creates and removes only a new private fixture. `mkfs.btrfs --rootdir`
builds it directly from a directory; `btrfs restore` reads it without a host mount.
Do not replace the temporary image path with an existing disk.

```sh
tools=/home/core/build-artifacts/spacewave-os-qemu-tools/bundle
fixture=$(mktemp -d "$tools/image-check.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
mkdir "$fixture/root" "$fixture/restored"
printf 'spacewave-rootdir-check\n' > "$fixture/root/known.txt"
truncate -s 256M "$fixture/fresh.btrfs"
"$tools/bin/mkfs.btrfs" --rootdir "$fixture/root" "$fixture/fresh.btrfs"
"$tools/bin/btrfs" restore "$fixture/fresh.btrfs" "$fixture/restored"
cmp "$fixture/root/known.txt" "$fixture/restored/known.txt"
```

## Packaging `/dev/console` without root

The kernel's [`usr/gen_init_cpio.c`](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/usr/gen_init_cpio.c?h=v6.17)
writes `newc` records from a text list, including device entries. A future
initramfs packager can use the matching kernel tree's host `gen_init_cpio` tool
with these entries:

```text
dir /dev 0755 0 0
nod /dev/console 0600 0 0 c 5 1
```

The tool encodes major/minor and ownership in the archive; it does not create a
host device node. This avoids `mknod` and root on the host. GNU cpio's `--owner`
alone cannot invent a missing device node. Building or integrating
`gen_init_cpio` is separate work; this bundle does not change the existing
root-only initramfs helper.

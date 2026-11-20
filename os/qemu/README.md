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

Forge lacks user KVM access. Check actual guest boot separately from extraction. The bundle
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

The [rootless archive builder](initramfs.md) uses Linux's `gen_init_cpio`
tool to encode `/dev/console`, ownership and permissions without host device
nodes. It streams the compressed archive and refuses existing outputs. The
bundle's GNU cpio is useful for inspection; `cpio --owner` alone cannot invent
a missing device node.

## Forge-managed application test

The [Gitea application proof](application.md) uses a separate development tool
image and explicitly granted KVM access. It checks the complete repository,
issue, build and VM replacement workflow through Flowgraph and Forge, while
retaining separate SQL, file and configuration bindings.

## Kill trial

`kill_trial.py` checks that the console guest keeps every record it
acknowledged. It takes the kernel, the initramfs and a seeded disk, and drives
the guest's serial console through QEMU's stdio, so it runs in the tool image
as well as on a KVM host. The guest writes 16 records of 256 KiB, each followed
by `sync`, and acknowledges each with its SHA-256. The driver kills QEMU with
SIGKILL at the first host disk write of a later 256 MiB write. It then reboots,
checks every acknowledged record, repeats the write and the kill, and checks
all 32 records before a clean poweroff. It prints `KILL_TRIAL_PASS` and exits 0
only if every check passes, and streams the guest's console to stdout.
`rehearse.sh` runs it on the rehearsal's disk after the clean boot. A 1 GiB
disk falls below the data Volume's disk reserve, so use 4 GiB.

## Revision trial

`revision_trial.py` checks that a guest moves to a staged revision and rolls a
broken one back, with its data intact. `rehearse.sh` seeds two image stores
under `/staging` on the rehearsal's disk, each with the rehearsal key's
authorization: `good`, which differs from the image by one file, and `broken`,
seeded by an init one byte longer than the boot set's, so its image check
fails. In each boot that reaches a shell, the guest mounts its own disk a second
time and runs `/workload stage` beside the console's read-only reader, then
`reboot`. The driver requires seven boots on the disk the kill trial left, two
of them cut by SIGKILL to QEMU as a power cut would: the accepted image starts
staging `good` and is cut at its first disk write; the next boot still serves
the accepted image and stages `good`; `good` boots on trial and is cut once
marked attempted; the next boot rolls it back and restages it, copying no
block; `good` boots on trial, is accepted and stages `broken`; `broken` boots
on trial, fails and reboots; the last boot rolls it back and serves `good`.
Each shell must still read the probe the first boot wrote in `/data`. It
prints `REVISION_TRIAL_PASS` and exits 0 only if every check passes.

## Refusal trial

`refusal_trial.py` checks that a boot refuses an image whose authorization no
pinned signer made. `rehearse.sh` builds its disk from a fresh copy of the
image, carded by a second throwaway key, with the first card's signer already
pinned in the data directory, as a card rewritten after first boot would be.
The guest must print `SPACEWAVE_OS_ERROR` naming the unpinned signer and never
print `SPACEWAVE_OS_AUTHORIZED`, `SPACEWAVE_OS_FROZEN` or
`SPACEWAVE_OS_DATA_CREATED`. A failed bootstrap pauses PID 1, so the driver
stops QEMU once the error appears; it prints `REFUSAL_TRIAL_PASS` and exits 0
only if every check passes.

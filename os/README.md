# Retained UnixFS boot experiment

Linux boots one static executable as PID1 from an independent initramfs. The
executable opens a retained s4db image read-only on a Btrfs virtual disk and serves
its selected immutable World transaction through `db/unixfs/fuse`.
It first starts an independent bootstrap copy of itself, which chroots and execs
`/workload`. The child hashes a lazy shared 3 MiB mmap and requires five mutations
to return `EROFS`. It exits before FUSE unmount, server join, snapshot release,
storage close, backing-filesystem unmount and poweroff.

The same executable implements bazil's privileged mount-helper FD handshake. A
private bootstrap symlink satisfies its `fusermount3` lookup. The initramfs holds
exactly one userspace ELF; matching kernel modules are kernel content. It contains
neither a distro mount helper nor a shell. Never install the executable setuid.

## Build and focused checks

Use the project's remote-build SOP, at most four workers, with one heavy job at a
time. From the repository root on the Linux build host:

```sh
make -f os/Makefile OUT=/path/to/artifacts GOSTYLE=/path/to/gostyle build check test model
```

`test` runs `TestRootPinsRevision`, `TestPersistentRootReopen` and
`TestDataVolumeReopen`. The retained-image case seeds real storage, verifies two
closes/reopens with identical Volume ID, root and
bytes, cancels the opening context while real readers remain live, and checks
storage stays usable until explicit close. It attempts an atomic publication with
an absent root and checks the earlier head survives. It then poisons the disposable
head and separately removes an actually read payload descendant while leaving the
root and retention intact. Both refusals preserve the database bytes, as do successful
reopens and cancellation. These are falsification checks, not a power-loss proof.
It also imports a tool input larger than the write buffer, checks exact content,
file modes, applet targets and empty mountpoints, rejects an interrupted import,
and removes a tool descendant to require a complete tool read before readiness.
Run that same compiled case with a fresh `TMPDIR` on the test Btrfs image when
checking the complete physical stack.

`model` keeps the one-processor direct-exec regression and adds the persistent
seed/fence/publication/reopen lifecycle, including cancellation, server failure,
child containment, server join and snapshot release before storage close.
Head-before-block, missing-retention, independent-closer, writable-serving and
unchecked-tool mutants must fail. Backend flush behavior remains the existing
`db/s4db/Durability.tla` contract; `db/volume/block/RootPublication.tla` covers the
blocks-before-head boundary separately.

## Seed once, boot twice

Use only new disposable paths on an isolated development VM, after any other
QEMU/storage experiment ends. Leave its ordinary boot disk, bootloader and SSH
unchanged. These host commands use distro tools; none enter the initramfs.
Choose new unique `BOOT` and `STORAGE` directories and place the built `init`,
these two scripts and the matching `vmlinuz` in `BOOT` first.

```sh
# STORAGE must be a newly created private directory, never an existing image.
fallocate -l 1G "$STORAGE/btrfs.img"
mkfs.btrfs -q "$STORAGE/btrfs.img"
mkdir "$STORAGE/mount"
mount -t btrfs -o loop,noatime "$STORAGE/btrfs.img" "$STORAGE/mount"
GOMAXPROCS=1 "$BOOT/init" seed "$STORAGE/mount/image" /path/to/static-busybox > "$BOOT/seed.log" 2>&1
umount "$STORAGE/mount"
```

`seed DIR STATIC-BUSYBOX` creates an absent directory and refuses an existing one,
including a partial seed. The library entry is
`boot.SeedRoot(ctx, logger, dir, executableReader, busyboxReader)`; both readers
belong to the caller. It streams this executable, the payload and BusyBox through
ordinary durable World commits, fences the engine, then publishes one generated
`world_block_engine.HeadState` through the tracked bucket's
`block.AtomicPublisher.PublishAtomic`. The `world-head` ObjectStore key uses the
controller's existing encoding and retained-root name. The atomic publication
also retains `selected-image`, so later unrelated head changes cannot reclaim it.
The publication waits for durability and releases its receipt pin. There is no
parallel manifest, JSON schema or boot-time fixture regeneration.

Every newly retained image requires `/workload`, `/payload` and `/bin/busybox`
as regular files with mode 0755. `/bin` has mode 0755 and holds relative symlinks
to `busybox` for this fixed applet interface:

```
sh cat ls echo pwd mkdir rmdir rm cp mv touch sync dd wc head tail sha256sum
sleep ps kill true false uname id stat mount umount
```

The caller supplies a static Linux BusyBox build implementing those applets.
Seeding accepts an input stream; it does not execute BusyBox or certify its ELF,
static linking, target architecture or applet support. Packaging and the later
QEMU console run must establish those properties. `/data`, `/dev`, `/proc` and
`/tmp` are empty mode-0755 mountpoints. The later scratch mount sets its own
permissions; UnixFS currently preserves only ordinary permission bits. This step
supplies mountpoints, not mounted storage or an interactive supervisor.
The diskless probe keeps its original two-file image. Older retained two-file
fixtures now fail completeness checks; they are never upgraded or reseeded.

Boot owns one read-only s4db handle from inspection through serving and close.
Missing files, unknown key layouts, missing heads/roots and missing retention are
refused without migration or reset. Before serving, it compares the complete probe
and payload against independent bootstrap inputs, checks every required node and
link, and streams all BusyBox content against its immutable file length. The
selected content-addressed World fixes tool bytes without another manifest or a
runtime dependency on a bootstrap BusyBox. The ordinary bare `store_kvtx` and World
cursor compose an immutable transaction without a writable Volume, controller or
tracked reader-pin writes. This isolated experiment has no writer or collector;
the existing durable selected root protects its descendants. Caller cancellation
contains the child but leaves readers and storage alive through ordered cleanup.
A server exit disconnects FUSE and cancels the child before that cleanup. Each boot prints the retained Volume ID, exact World
reference, byte counts and SHA-256 hashes. The script compares these with the
seed's independent close/reopen readback, rather than accepting an equivalent
new fixture.

Package matching uncompressed `.ko` files in dependency order. For Debian, obtain
the paths with `/usr/sbin/modprobe --show-depends virtio_blk` and
`/usr/sbin/modprobe --show-depends btrfs`, deduplicate while preserving order, and
decompress their `.xz` files on the outer host. Pass any modular FUSE dependency
as well. `make-initramfs.sh` prefixes the supplied module filenames by order; PID1
loads at most sixteen using `finit_module`, without `modprobe` or another binary.
The tested Debian kernel has built-in FUSE and virtio PCI, with eight supplied
modules for virtio block and Btrfs. This is bounded experiment wiring, not general
hardware discovery or provisioning.

```sh
# MODULES below is an explicit dependency-ordered list of matching .ko paths.
sh "$BOOT/make-initramfs.sh" "$BOOT/init" "$BOOT/initramfs.cpio.gz" $MODULES
sh "$BOOT/boot-qemu.sh" "$BOOT/vmlinuz" "$BOOT/initramfs.cpio.gz"   "$BOOT/boot1.log" "$STORAGE/btrfs.img" "$BOOT/seed.log"
sh "$BOOT/boot-qemu.sh" "$BOOT/vmlinuz" "$BOOT/initramfs.cpio.gz"   "$BOOT/boot2.log" "$STORAGE/btrfs.img" "$BOOT/seed.log"
```

Run packaging as root for `/dev/console`; use root for host mount/seed commands.
Unmount and detach any host loop device before QEMU opens the same image. QEMU
attaches only this regular file through virtio block with `cache=none`; networking,
graphics and host-directory sharing stay absent. PID1 mounts `/dev/vda` as Btrfs at
bootstrap `/store` and opens `/store/image`. It never mounts the outer Debian disk.
The guest needs initramfs, procfs, devtmpfs, FUSE, ELF and serial support.

`boot-qemu.sh` uses one vCPU, 768 MiB RAM and `GOMAXPROCS=1` for PID1 and workload.
It asserts both actual processor values, exact retained identity/content and every marker
through clean kernel poweroff. KVM is the default; `ACCEL=tcg` selects emulation.

## Baseline and hosted failure cleanup

The diskless three-argument QEMU command remains supported. Without
`SPACEWAVE_OS_STORE`, PID1 builds the earlier in-memory image. A hosted process
unshares its mount namespace while locked to its OS thread through all cleanup:

```sh
sudo env GOMAXPROCS=1 "$BOOT/init" probe
sudo env GOMAXPROCS=1 "$BOOT/init" probe-fail # must fail and still unmount
sudo env GOMAXPROCS=1 "$BOOT/init" probe /path/to/retained/image
sudo env GOMAXPROCS=1 "$BOOT/init" probe-wait /path/to/retained/image
```

`probe-wait` runs the same mmap and EROFS checks, then prints `WAITING` while its
mapped workload stays live. Send SIGTERM to the supervisor PID, not the child.
Require nonzero exit, absence of PASS, and ordered CHILD_JOINED, UNMOUNT_OK,
SERVER_JOINED, SNAPSHOT_RELEASED and STORAGE_CLOSED markers. Compare the calling
host's namespace and mountinfo before/after, and hash the retained database across
the probe. The ordinary 45-second supervisor deadline bounds a forgotten probe.

Keep bootstrap executable/runtime pages and physical storage outside the served
tree. Direct vfork exec through the server's own FUSE tree can retain its sole Go
processor while the kernel needs that server. The independent bootstrap exec is
required, even though the workload executable has the same bytes.

## Limits

This is a bounded immutable-image experiment, not a distribution or service
registry. Chroot is not a security sandbox: the child inherits serial descriptors
and root credentials. PID1 supervises one foreground workload; Flowgraph remains
the authority for future managed work. PID1 retains bootstrap `/`, with `/proc`,
`/dev`, `/tmp` and physical storage outside the workload's UnixFS root.

Two clean nested boots and an incomplete-root rejection do not prove provider
power-loss durability. Linux barriers pass through nested virtio, QEMU, a regular
file on the outer ext4 filesystem, virtualization and provider caches. Guest cache
loss does not evict every host/provider cache. Validation warms database data;
lazy workload mmap proves page-fault service, not a cold-media benchmark. The small
image fits RAM. There is no Btrfs speed, general crash-safety or large-data claim.
Authenticated image selection/enrollment, writable roots, ordinary application
compatibility, provider-native boot installation and production supervision remain
outside this experiment.

# Retained UnixFS boot experiment

## Serial console runtime

The guest-only console runs `/bin/sh -i` from the verified frozen image with a
separate writable UnixFS `/data`. PID1 stays in the independent initramfs. The
shell starts in `/data` and gets the real `/dev/ttyS0` controlling terminal,
inherited input/output, Ctrl-C and normal shell job control. `/proc` is procfs,
`/dev` is devtmpfs with a nested private devpts mount, and `/tmp` is mode-1777
tmpfs. Scratch files disappear on shutdown; application files belong in `/data`.
This is trusted root access in an isolated guest, not a security sandbox.

Create a data filesystem explicitly for a **fresh disposable fixture** with
`spacewave-init seed-data ABSENT-DIRECTORY`. This delegates to `boot.CreateData`
and closes the result before printing `SPACEWAVE_OS_DATA_SEEDED`. It refuses
existing directories, including partial creations. The existing image seed
command is unchanged. Use the same built init to seed `/workload` and boot: the
complete image check compares that file with the independent init executable.
Runtime calls only `OpenRoot` and `OpenData`; unknown or incomplete stores are
refused without reset or reseeding.

After preparing fresh matching kernel/initramfs/disk inputs separately, launch
from a real terminal or PTY:

```sh
ACCEL=tcg sh os/qemu/run-console.sh /path/to/vmlinuz /path/to/initramfs.cpio.gz /path/to/fresh-btrfs.img
```

The launcher consumes those existing files. It does not create fixtures, mount
anything on the host, download tools or start a background service. The disk must
be detached from host mounts and other VMs and contain `/image/image.s4wave`
and `/data/data.s4wave`. `QEMU` selects an existing QEMU executable (default
`qemu-system-x86_64`); `ACCEL` is `kvm` (default) or `tcg`. It uses one vCPU,
768 MiB, virtio raw disk with `cache=none`, serial/monitor stdio multiplexing,
and disabled networking/graphics. Ctrl-C is serial input (`signal=off`), not a
host QEMU interrupt. **Ctrl-a x exits QEMU**, even on failed startup; Ctrl-a c
switches between serial and the QEMU monitor. Abrupt QEMU exit is not graceful
shutdown. No crash-durability result is implied by this launcher.

The exact guest interface is `/init` with these kernel-command-line environment
assignments (or `/init console` with the same store assignments):

```text
GOMAXPROCS=1
SPACEWAVE_OS_MODE=console
SPACEWAVE_OS_STORE=/store/image
SPACEWAVE_OS_DATA=/store/data
SPACEWAVE_OS_TTY=/dev/ttyS0
SPACEWAVE_OS_NATIVE_BENCH=0
```

The launcher also passes `console=ttyS0,115200 rdinit=/init panic=-1 quiet`. Bootstrap
mounts `/dev/vda` as Btrfs at `/store`, outside both served roots. The library API
is `boot.RunConsole(ctx, shutdown, systemDir, dataDir, terminalPath, nativeBench)` and
refuses every non-PID1 invocation before touching storage or mounts. Hosted
`probe`, `probe-fail` and `probe-wait` keep their private mount namespace and
bounded `exec.Cmd` lifecycle; they never enter guest-wide waiting or signalling.

For a later native comparison only, `NATIVE_BENCH=1` tells the launcher to pass
`SPACEWAVE_OS_NATIVE_BENCH=1`. The disk must already contain a private mode-0700
`/native-bench` directory, separate from both database directories and holding
only comparison files. Runtime binds only `/store/native-bench` at
`/tmp/native-bench`; it rejects a missing directory or symlink. It never exposes
the backing database directories through a bind mount and runs no benchmark.

Finish foreground and background jobs, then run `poweroff` for graceful shutdown.
BusyBox sends SIGUSR2 to PID1. This requests ordinary cleanup without cancelling
healthy FUSE service; a cancelled service would prevent buffered writes from
flushing. `sync; exit` (or Ctrl-D after syncing) also shuts down. PID1 kills
remaining guest processes, including detached
sessions, and reaps adopted orphans to `ECHILD`. A single Wait4 authority starts
after every mounting helper has joined; it never races `exec.Cmd.Wait`. The
shell starts through an independent bootstrap exec before chroot/exec into
FUSE, preserving the one-processor self-deadlock guard. PID1 then unmounts nested
kernel/data/system mounts, joins both FUSE servers, explicitly releases both
World cursors and their cached children, then closes each World and physical
store, unmounts `/store`, and powers off. A failed ordinary
unmount remains an error even if detached cleanup succeeds. If reaping cannot
prove all users stopped, stores stay open and bootstrap stays in recovery.

Server exits and controller stops cancel the session and disconnect FUSE before
containment. Physical storage still outlives its users. Data's buffered WriteAt
also watches controller availability, including idle/release flushes whose
callers use background contexts; Close reports the failure rather than waiting
forever for an unavailable writer. Close errors remain visible. A failed console
prints the cause and recovery instructions and leaves PID1 stopped: use Ctrl-a x,
check the reported fixture/input error, and rerun the same launcher command.
There is no automatic repair, shell restart or fallback to a new filesystem.

`GuestConsole.tla` covers guest-only signals, one waiter, child/server/controller
failure, containment, mount/server joins and store lifetime. `DataVolume.tla`
covers interrupted buffered flush and storage publication; `Console/Console.tla`
and its view carry the reader journey. Component tests and model checks do not
establish mounted durability or actual terminal behavior. Real QEMU/PTY checks,
80x24 and 160x48 captures, restart/crash acceptance and timings remain separate.

## Immutable probe

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

Prepare a new disposable fixture on the Linux build host. Install the private
[host tools](qemu/README.md) and [rootless archive tool](qemu/initramfs.md)
first. Keep the built static init, matching kernel and modules outside the
served filesystems. These commands need no root access or host mounts:

```sh
tools=/absolute/private-tool-bundle
init=/absolute/built-init
fixture=$(mktemp -d /absolute/artifacts/interactive.XXXXXX)
mkdir -m 700 "$fixture/root" "$fixture/root/native-bench"
GOMAXPROCS=1 "$init" seed "$fixture/root/image" "$tools/root/usr/bin/busybox" \
  > "$fixture/seed.log" 2>&1
GOMAXPROCS=1 "$init" seed-data "$fixture/root/data"
truncate -s 1G "$fixture/btrfs.img"
"$tools/bin/mkfs.btrfs" --rootdir "$fixture/root" "$fixture/btrfs.img"
# Supply dependency-ordered, uncompressed modules for this exact kernel.
GEN_INIT_CPIO=/absolute/tool/gen_init_cpio sh os/make-initramfs.sh \
  "$init" "$fixture/initramfs.cpio.gz" /absolute/modules/*.ko
ACCEL=tcg sh os/qemu/run-console.sh /absolute/vmlinuz \
  "$fixture/initramfs.cpio.gz" "$fixture/btrfs.img"
```

`mkfs.btrfs --rootdir` imports the independently closed stores into a new disk
image. The ordinary runtime only opens them. Exit the guest, then rerun the
same launcher command for a clean restart. Never open one disk in two guests
at once. Use `NATIVE_BENCH=1` only when testing the prepared private comparison
directory. The frozen system rejects writes; keep application files in `/data`.

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
sleep ps kill true false uname id stat mount umount poweroff
```

The caller supplies a static Linux BusyBox build implementing those applets.
Seeding accepts an input stream; it does not execute BusyBox or certify its ELF,
static linking, target architecture or applet support. Packaging and the later
QEMU console run must establish those properties. `/data`, `/dev`, `/proc` and
`/tmp` are empty mode-0755 mountpoints. The later scratch mount sets its own
permissions; UnixFS currently preserves only ordinary permission bits. The image
seed supplies mountpoints; console runtime overlays them as described above.
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
# Optional bounded read-only probes, with the same prepared fixture above.
PATH="$tools/bin:$PATH" ACCEL=tcg sh os/boot-qemu.sh /absolute/vmlinuz \
  "$fixture/initramfs.cpio.gz" "$fixture/boot1.log" \
  "$fixture/btrfs.img" "$fixture/seed.log"
PATH="$tools/bin:$PATH" ACCEL=tcg sh os/boot-qemu.sh /absolute/vmlinuz \
  "$fixture/initramfs.cpio.gz" "$fixture/boot2.log" \
  "$fixture/btrfs.img" "$fixture/seed.log"
```

The archive builder encodes `/dev/console` without creating a host device.
Keep the disk detached from host mounts and other guests before QEMU opens it. QEMU
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
Authenticated image selection/enrollment, a writable system root, general
application compatibility, provider-native boot installation and production
supervision remain outside this experiment.

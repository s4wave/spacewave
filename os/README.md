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
At boot the runtime opens the image with `OpenRoot` and the data Volume with
`EnsureData`, which creates the Volume and its key only when the data
directory holds no store, as on first boot. Creation builds the store under a
temporary name and renames it into place, so a cut first boot leaves no store
or a complete one. Unknown or incomplete stores are refused without reset or
reseeding.

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

An image that holds `/usr/bin/spacewave` also runs the Spacewave daemon beside
the shell, so the signed image decides whether a Device runs one. PID1 binds
the daemon's state directory, `daemon/` beside `data.s4wave` on the backing
store, at `/var/lib/spacewave`, and the card's bootstrap grant read-only at
`/etc/spacewave/grant`; the image provides both mountpoints. The daemon starts
through the same independent bootstrap exec as the shell (`spacewave-init
enter-daemon ROOT [grant]`) and runs `spacewave serve --state-path
/var/lib/spacewave --idle-timeout 0`, with `--device-grant` when the card
holds a grant, so first boot enrolls the Device. It runs in its own session
and writes to the serial console. The single Wait4 authority reaps it and
restarts it after 2 s, doubling to 1 min, while the console runs. Shutdown
sends it SIGTERM, which `spacewave serve` handles as an external stop that
drains its clients and releases its state, and waits up to 10 s before
containment kills what remains (`SPACEWAVE_OS_DAEMON_STOP_TIMEOUT`). The
daemon's store acknowledges only durable commits, so SIGTERM is courtesy, not
a durability requirement. `os/qemu/package_daemon.py` writes the image
assets for this: a static `spacewave`, the CA bundle and the mountpoints.

With `ip=dhcp` on the kernel command line and `CONFIG_IP_PNP_DHCP`, the kernel
leases an address before init. When the image holds `/etc/resolv.conf`, PID1
copies `/proc/net/pnp`, the lease's resolvers, into its initramfs and binds
the copy over it read-only; procfs net entries cannot be bind sources. The kernel
never renews the lease, which suits QEMU's user network and a LAN with long
leases; a DHCP client in the image would replace this when renewal matters.

Server exits and controller stops cancel the session and disconnect FUSE before
containment. Physical storage still outlives its users. Data's buffered WriteAt
also watches controller availability, including idle/release flushes whose
callers use background contexts; Close reports the failure rather than waiting
forever for an unavailable writer. Close errors remain visible. A failed console
prints the cause and recovery instructions and leaves PID1 stopped: use Ctrl-a x,
check the reported fixture/input error, and rerun the same launcher command.
There is no automatic repair, shell restart or fallback to a new filesystem.

`GuestConsole.tla` covers guest-only signals, one waiter, child/server/controller
failure, containment, mount/server joins, store lifetime, and the daemon's
restarts and SIGTERM before containment. `DataVolume.tla`
covers interrupted buffered flush and storage publication; `Console/Console.tla`
and its view carry the reader journey. `OSEnroll.tla` covers an installed card's
first boot: the signers pinned before any root is served, the Device key made in
the Volume, and the bootstrap grant redeemed across power cuts until the Space
admits the key or revokes the grant. `OSRevision.tla` covers moving between
system revisions: the image store's accepted, trial, attempted and rolled-back
roots, each move one atomic publish; a trial marked attempted before it is
served, so a failed or cut trial boots back to the accepted revision once;
each revision checked against its own authorization; a rolled-back revision
never restaged; and collection that keeps both revision roots. Component tests and model checks do not
establish mounted durability or actual terminal behavior. Real QEMU/PTY checks,
80x24 and 160x48 captures, restart/crash acceptance and timings remain separate.

## SkiffOS boot set

`os/skiff/spacewave/os` is a SkiffOS config package. Added last to a board's
`SKIFF_CONFIG`, with `SKIFF_EXTRA_CONFIGS_PATH` naming a copy of `os/skiff`, it builds
that board's boot set with `spacewave-init` as the initramfs's only program.
Its kernel fragment builds in Btrfs, FUSE and the other filesystems the init
mounts, so the initramfs carries no modules. Its post-image hook packs the init
named by `SPACEWAVE_OS_BOOT/init` with `SPACEWAVE_OS_BOOT/make-initramfs.sh`, a
copy of `os/make-initramfs.sh`, and the kernel
tree's own `gen_init_cpio`, writes it over `rootfs.cpio.lz4`, the file every
board's scripts load, and removes the uncompressed Buildroot root. The package
turns off the U-Boot ramdisk wrapper, so a U-Boot board needs one added.

The Forge Step `forge/lib/skiffos` runs that build. Its `input_env` checks the
boot input, a UnixFS directory holding `init` and `make-initramfs.sh`, out of
the World and sets `SPACEWAVE_OS_BOOT`, and its `extra_config_inputs` checks out
`os/skiff` the same way. The build pins the SkiffOS checkout by
`skiff_revision` and refuses one at another commit or with local changes, so
the Task's reuse key covers every byte the build reads.

An image Step runs `spacewave-init seed` in the `os/qemu` tool image and
stores the image store as a UnixFS tree; the rehearsal and the install both
read that one tree, so the root a Space owner signs is the root the rehearsal
booted and the install writes.

A boot set is offered only after a rehearsal boots it. `os/qemu/rehearse.sh`
copies the image store into a fresh system root, authorizes it with a
throwaway signer (`spacewave-init rehearsal-card`), seeds two more revisions
signed by the same key under `staging/`, and writes the root at its
content's size (`mkfs.btrfs --shrink`) onto a 4 GiB disk. It leaves the data
Volume absent, as an install does. It boots the boot set on that disk with
`run_app.py --dhcp`, on QEMU's user network with `ip=dhcp`; first boot grows
the store to the disk, pins the signer, verifies the authorization, and only
then creates the data Volume and its key.
The guest writes and syncs one file through the data Volume and powers off;
an image with a resolv.conf also prints `SPACEWAVE_OS_RESOLVER_BOUND`, and an
image with the daemon prints `SPACEWAVE_OS_DAEMON_LISTENING` once its socket
appears. The rehearsal then runs the [kill trial](qemu/README.md#kill-trial) and the
revision trial on the same disk, and the [refusal trial](qemu/README.md#refusal-trial) on a copy whose
card an unpinned signer authorized. It writes its receipt only when the guest
printed `SPACEWAVE_OS_BACKING_GROWN`, `SPACEWAVE_OS_AUTHORIZED`,
`SPACEWAVE_OS_DATA_CREATED` and `SPACEWAVE_OS_PASS`, the kill trial kept every
acknowledged record, the [revision trial](qemu/README.md#revision-trial)
moved to a staged revision and rolled a broken one back, and the copy refused
to boot. The receipt is a directory
whose `root` file names the root the guest authorized and booted; a
guest that panics or hangs fails, and its console is the Step's log. The
rehearsal runs as a Docker Step in the `os/qemu` image with `/dev/kvm`. The
Step's `input_mounts` check out the boot set, the image, the init and the
scripts read-only.

### Revisions

The image store holds each revision with its own authorization, under the
named roots `OSRevision.tla` models (`os/boot/revision.go`). A revision root
names an authorization block: the signed image root, stored with that root as
its one child, so one named root holds the revision's whole block graph and one
publish moves a revision with its authorization. `accepted` is the revision the
Device falls back to; `trial` is a staged revision to boot once; `attempted`
holds the trial once a boot has tried it; `rolled-back` records the image root
of the last trial that failed, in a block with no children, so it keeps none of
that revision. Each move is one `SetBucketRoots` commit whose `Previous` values
make it a compare-and-swap.

A boot verifies both slots against the pinned signers. A pending, authorized
trial not yet attempted is marked `attempted`, durably, and then served
(`SPACEWAVE_OS_TRIAL`). A trial already attempted, or one no pinned signer
authorized, is dropped and recorded as rolled back
(`SPACEWAVE_OS_ROLLED_BACK root= reason=`), and the boot serves `accepted`. An
ordinary boot writes no byte of the store. On trial, the console accepts the
revision once its daemon rewrites `spacewave.sock.ready` in its state
directory, or at once for an image without the daemon
(`SPACEWAVE_OS_ACCEPTED`). The accept runs as a child,
`spacewave-init accept-trial`, because the console's process already holds
the store's one read-only handle and s4db opens a file once per process. It reboots when a five-minute deadline, counted from
selection, passes first, and on any error before acceptance, so the next boot
rolls back. A deadline that passes before the shell starts syncs and restarts
the machine at once, since a stalled boot step never returns to the console. BusyBox
`reboot` (SIGTERM to init) closes every store as poweroff does and restarts.

`spacewave-init stage IMAGE SOURCE AUTHORIZATION` stages the image a seed built
in SOURCE as the trial. It opens the store as its one writer beside the
console's read-only reader and copies the source root's graph with
`block.CopyGraph`, which stops at every block the store already holds and
writes each block after its children; a buffer commits those writes in order,
in batches. A held block therefore always holds its whole graph, so a second
staging copies nothing and a staging cut short keeps what it completed and
resumes. The source is any block store that serves each block's child list,
so the same staging reads a second image store here and a Space's bucket on a
Device. The trial
and its authorization publish in one commit, refused while another trial is
pending. The rehearsal's [revision trial](qemu/README.md#revision-trial) runs it
in a booted guest.

`os/qemu/install.sh` writes the installable disk. It copies the image store
into a fresh system root and runs `spacewave-init install-card` with the
Space's signers and bootstrap grant and the owner's authorization. The command
refuses an authorization no listed signer made, one naming another root, and a
grant that is unsigned, aimed at a peer, or expired. It writes the signers and
grant before the authorization, so a cut install leaves no card that boots.
The script writes `disk.img` at its content's size, which first boot grows to
the disk it lands on, with no data Volume. Beside the disk it places the boot
set's `bzImage` and `rootfs.cpio.lz4`. A QEMU guest boots the kernel directly,
so the boot set is the VM's boot adapter and stays outside the disk.

The `os/image/sign` Step authorizes a rehearsed image. It takes the image
store and the rehearsal's receipt, reads the store's selected root with
`boot.ReadImageRoot`, refuses it unless the receipt names the same root, and
asks the holder of its Worker's key to sign it
through the `SignWithPeer` directive. On a Device the running Session answers,
and the Space runtime's bridge forwards the request only for that Session's
peer, so the key never reaches the Step. Its output is the authorization tree
install mounts. Place the Step on the Space owner's Device, whose peer the
cards pin.

`TestSpacewaveOSRehearsal` in `sdk/forge/world` builds the QEMU board with the
package and checks that the stored initramfs holds the init alone. An image
Step seeds the image, and connections carry the boot set and the image to the
rehearsal Step, which must pass, and the image and the receipt to the sign
Step. The test installs the Step's authorization with a card naming the
Worker; the install must succeed, and fail with `is not pinned` for another
key's authorization. A last graph rehearses the same kernel with an initramfs
that is not an archive, which must fail with the kernel's panic in its log and
never start its sign Step. The test needs a SkiffOS
checkout (`SPACEWAVE_SKIFFOS_DIR`), a static init (`SPACEWAVE_OS_INIT`), the
tool image (`SPACEWAVE_QEMU_IMAGE`), the Docker CLI (`SPACEWAVE_DOCKER`), and a
`DOCKER_HOST` whose daemon shares this host's filesystem. A Docker Step that
writes outputs runs as the Worker's user, with the group of each device it
maps, so the Worker owns what the container wrote.

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

`model` checks every spec under `os/boot` with `aptre-tla`: the one-processor
direct-exec regression, the persistent seed/fence/publication/reopen lifecycle,
including cancellation, server failure, child containment, server join and
snapshot release before storage close, the data Volume, the guest console and
first-boot enrollment. Each `Foo.mutants.json` lists the mutants that must break
a named rule, such as head-before-block, missing retention, an independent
closer, a writable served root and unchecked tools; a mutant that selects a
design alternative names its `Foo-<mutant>.cfg.refuted`. Backend flush behavior
remains the existing `db/s4db/Durability.tla` contract;
`db/volume/block/RootPublication.tla` covers the blocks-before-head boundary
separately.

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
truncate -s 4G "$fixture/btrfs.img"
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
also retains the built root as the named root `image`, so later unrelated head
changes cannot reclaim it, until a card's authorization moves it to `accepted`.
The publication waits for durability and releases its receipt pin. There is no
parallel manifest, JSON schema or boot-time fixture regeneration.

Every newly retained image requires `/workload`, `/payload` and `/bin/busybox`
as regular files with mode 0755. `/bin` has mode 0755 and holds relative symlinks
to `busybox` for this fixed applet interface:

```
sh cat ls echo pwd mkdir rmdir rm cp mv touch sync dd wc head tail sha256sum
sleep ps kill true false uname id stat mount umount poweroff reboot ip su chmod
stty
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

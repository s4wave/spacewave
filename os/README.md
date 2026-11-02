# Immutable UnixFS boot experiment

Linux boots one static executable as PID1 from an independent initramfs. It
constructs a real in-memory Volume, World and UnixFS image, pins a read transaction,
and serves that revision through `db/unixfs/fuse`. It first starts an independent
bootstrap copy of the same executable, which chroots and execs `/workload`.
That child hashes a lazy shared mmap of a 3 MiB file and checks
that five mutations return `EROFS`. The child exits before unmount, server join,
snapshot release and poweroff. The same executable implements bazil's privileged
mount-helper FD handshake. A temporary bootstrap symlink satisfies bazil's
`fusermount3` lookup; a distro helper is not included.

## Run on Linux

Use the project's remote-build SOP. Run from the repository root:

```sh
make -f os/Makefile build check test model
sudo /home/core/build-artifacts/spacewave-os/init probe
sudo /home/core/build-artifacts/spacewave-os/init probe-fail # expected exit 1
sudo sh os/make-initramfs.sh /home/core/build-artifacts/spacewave-os/init /tmp/os.cpio.gz /path/to/fuse.ko
sh os/boot-qemu.sh /path/to/vmlinuz /tmp/os.cpio.gz /tmp/os-serial.log
```

Use the uncompressed `fuse.ko` from the exact kernel build, or omit it if FUSE is
built in. The kernel needs initramfs, procfs, devtmpfs, FUSE, ELF and serial console
support. Nested QEMU defaults to KVM; `ACCEL=tcg` selects software emulation. It
attaches neither a disk nor a network. The probe unshares its mount namespace.
Both modes require root; the helper must never be installed setuid.

`boot-qemu.sh` is the retained real regression: it uses one virtual CPU, passes
`GOMAXPROCS=1` to PID1 and checks its actual value before accepting exec through
poweroff. `make model` checks the bootstrap design and requires the retained
direct-exec mutant to fail `EventuallyStopped` with an unavailable server.

## What this proves and what remains

This is a bootstrap and kernel-adapter experiment, not a general distribution.
The root contains just the executable and payload. Workload chroot is not a
security sandbox: it inherits serial descriptors and root credentials. PID1
supervises one bounded foreground child; it is not a general init or service
supervisor. Direct vfork exec from the server's own FUSE mount can starve its
server with one Go processor; the independent bootstrap exec avoids that cycle.
Startup errors stop on serial for recovery; workload failures clean
up and power off with a failure marker. `Boot.tla` models these bounded phases
and the ordering of child, mount and snapshot release.

The server executable, its runtime pages and physical Volume never reside on the
filesystem they serve. Bootstrap `/proc`, `/dev` and `/tmp` remain outside UnixFS;
the workload's root is the immutable FUSE tree, while PID1 retains bootstrap root.
The Volume is rebuilt in memory on each boot. Persistence, authenticated image
selection, ordinary applications, writable state, recovery UI, boot-disk install
and production service management remain unproved and intentionally absent.

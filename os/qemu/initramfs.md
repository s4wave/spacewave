# Rootless prototype initramfs

`os/make-initramfs.sh` uses Linux's `usr/gen_init_cpio` host tool. It does not
need root, `mknod`, cpio, a kernel build or host mounts. Build the tool once in
a fresh private prefix on the build host:

```sh
sh -n os/make-initramfs.sh
sh -n os/qemu/build-initramfs-tool.sh
sh os/qemu/build-initramfs-tool.sh /absolute/fresh-initramfs-tool
GEN_INIT_CPIO=/absolute/fresh-initramfs-tool/gen_init_cpio \
  sh os/make-initramfs.sh /absolute/static-init /absolute/new.cpio.gz \
  /absolute/first.ko /absolute/second.ko
```

The tool builder needs curl, sha256sum and a host C compiler (`CC`, default
`cc`). It downloads the unchanged primary source from Torvalds' Linux v6.17
tag at
`https://raw.githubusercontent.com/torvalds/linux/v6.17/usr/gen_init_cpio.c`,
SHA256 `309cab96d2b43133a0758f165fbcf9cc1b671623555ef8dadd9983095c462345`.
It retains that source beside the host executable for inspection or rebuilding;
neither belongs in Git. A failed build leaves its fresh prefix for inspection.

The archive builder needs that executable (`GEN_INIT_CPIO`, default
`gen_init_cpio` on PATH), gzip, realpath and ordinary POSIX shell utilities.
Its positional interface is unchanged: static init, fresh output, then zero to
16 **uncompressed modules matching the boot kernel, in dependency order**.
Module basenames may contain ASCII letters, digits, `_`, `.`, `+` and `-`.
It preserves their order through numeric archive filename prefixes.

The output's parent directory must already exist. Existing output files,
directories and symlinks are rejected. Private staging is created beside the
output and removed on exit. Both generator and gzip status are checked before
an atomic, no-clobber hard link publishes the completed archive. Generation
streams through a FIFO; it does not collect the archive in memory or on disk.
The staged input copies and compressed output use disk space proportional to
the supplied files.

The kernel text spec encodes root ownership, `/init` mode `0755` and
`/dev/console` character device `5:1` mode `0600`; it creates no host device
nodes. `gen_init_cpio -t 0` fixes entry mtimes and `gzip -n -1` omits gzip
timestamps and source filenames. The same input bytes and module order produce
the same output with the same tool versions, regardless of input mtimes or
host umask.

Boot acceptance must use the matching kernel and modules, a fresh serial log,
and the existing bounded `os/boot-qemu.sh`. Its three-argument mode runs the
prototype's in-memory self-test without a backing disk; that check does not
establish persistent-storage recovery.

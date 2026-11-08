# Native ext4 and Btrfs loopback baseline

Run only on the isolated disposable VM, with other experiments stopped:

```sh
sudo python3 compare.py --isolated-vm
sudo python3 compare.py --isolated-vm --reopen /srv/spacewave-os/storage/compare-RESULT
```

The program formats only exclusive new regular files in a new `compare-*`
directory. It never accepts a device to format. Each 1 GiB image is preallocated
on the guest's ext4 root before timing. Three pairs alternate ext4/Btrfs order.
Mounts use `noatime`; compression is off, Btrfs retains checksums/CoW and duplicate
metadata, and ext4 retains its journal. Ext4 inode and journal initialization
finish before measurements. Formatting and snapshot creation are not timed.

Each filesystem receives the same 128 MiB deterministic blob and 1,000 small
files. Durable blob creation includes file fsync and parent directory fsync;
small-file timing includes both syncs per file. These are application-visible
durability requests, not proof of hardware honoring flushes or power-loss safety.
Cold stat and read follow `sync` and guest `drop_caches=3`; warm repeats immediately.
Reads include streaming SHA-256, so report CPU and syscall overhead as part of
the workload. The blob repeats a seeded 1 MiB pseudorandom chunk; neither filesystem
uses compression or deduplication. Images, results and known content stay on the VM.

The correctness check unmounts, detaches, reattaches and remounts every image,
then verifies the blob hash, all small-file contents and the changed live marker.
Btrfs additionally preserves the old marker and blob in a read-only snapshot;
a write to that snapshot must fail with EROFS. This tests reopen and snapshot
isolation, not a process crash, machine restart, power cut or Spacewave Volume.
`--reopen` checks retained data again without formatting. Keep images for the
boot worker's backing-store experiments. It must use different new image paths.

## Limits and cleanup

Both filesystems run over loop devices, an outer ext4 filesystem, a virtual disk
and unknown provider storage/cache. Guest cache dropping cannot clear provider
caches. The small workload fits RAM and does not establish sustained bandwidth,
tail latency, production sizing or a universally faster filesystem. Btrfs CoW and
checksums do more work than ext4. This is **not UnixFS performance**; measure the
boot worker's actual Volume/FUSE/exec path separately.

The mount context releases the filesystem before detaching its own loop device.
After interruption, inspect `findmnt` and `losetup -j IMAGE` first. Unmount only
`compare-RESULT/mount`, then detach only the loop belonging to that image. Never
use `losetup -D`. Once all six images are unmounted and detached and their results
are saved, delete exactly that `compare-RESULT` directory to recover about 6 GiB.
Deleting files does not stop VM billing; terminate the VM through the provider
when the experiment is finished.

#!/usr/bin/env python3
"""Compare native filesystems in newly created loop images on an isolated VM."""

import argparse
from collections.abc import Iterator
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import platform
import random
import subprocess
import tempfile
import time


# These bounds fit a 2 GiB disposable VM and keep all formatting off physical disks.
STORAGE_ROOT = Path("/srv/spacewave-os/storage")
IMAGE_BYTES = 1024 * 1024 * 1024
FILE_COUNT = 1000
BLOB_MIB = 128


def run(*args: str) -> str:
    """Run one checked external operation with a bounded lifetime."""
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT, timeout=120)


def sync_directory(path: Path) -> None:
    """Make directory entries durable, separately from file contents."""
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def write_durable(path: Path, chunk: bytes, count: int) -> float:
    """Time creation, streaming writes, file fsync and parent directory fsync."""
    start = time.perf_counter()
    with path.open("xb", buffering=0) as output:
        for _ in range(count):
            view = memoryview(chunk)
            while view:
                written = output.write(view)
                if not written:
                    raise OSError("write made no progress")
                view = view[written:]
        os.fsync(output.fileno())
    sync_directory(path.parent)
    return time.perf_counter() - start


def hash_file(path: Path) -> str:
    """Read and hash a file with bounded memory."""
    digest = hashlib.sha256()
    with path.open("rb", buffering=0) as source:
        while block := source.read(1024 * 1024):
            digest.update(block)
    return digest.hexdigest()


def drop_caches() -> None:
    """Clear this isolated guest's caches; provider caches remain uncontrolled."""
    os.sync()
    Path("/proc/sys/vm/drop_caches").write_text("3\n", encoding="ascii")


@contextmanager
def mount_image(image: Path, target: Path, filesystem: str) -> Iterator[Path]:
    """Attach this image and release the mount before detaching its loop device."""
    loop = run("losetup", "--find", "--show", str(image)).strip()
    try:
        run("mount", "-t", filesystem, "-o", "noatime", loop, str(target))
        try:
            yield target
        finally:
            run("umount", str(target))
    finally:
        run("losetup", "--detach", loop)


def create_image(image: Path, filesystem: str) -> None:
    """Format only a newly created exclusive regular file, never a supplied device."""
    # Refuse replacement and preallocate the outer ext4 extents before timing.
    fd = os.open(image, os.O_CREAT | os.O_EXCL | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        os.posix_fallocate(fd, 0, IMAGE_BYTES)
        os.fsync(fd)
    finally:
        os.close(fd)

    # Complete ext4 initialization up front; retain each filesystem's safe defaults.
    if filesystem == "ext4":
        run("mkfs.ext4", "-q", "-E", "lazy_itable_init=0,lazy_journal_init=0", str(image))
    else:
        run("mkfs.btrfs", "-q", str(image))


def verify_tree(root: Path, expected_hash: str, snapshot: bool) -> None:
    """Verify known data after remount, including Btrfs snapshot isolation."""
    # Stream the known tree and reject missing or invented directory entries.
    live = root / "live"
    if hash_file(live / "blob") != expected_hash:
        raise ValueError("blob changed after reopen")
    count = 0
    for entry in (live / "small").iterdir():
        number = int(entry.name)
        if (not 0 <= number < FILE_COUNT or entry.name != f"{number:06d}"
                or entry.read_bytes() != f"entry {number}\n".encode()):
            raise ValueError(f"invalid metadata file: {entry}")
        count += 1
    if count != FILE_COUNT or (live / "marker").read_bytes() != b"after\n":
        raise ValueError("live tree did not reopen exactly")

    # A read-only snapshot must retain the pre-mutation marker and large content.
    if snapshot:
        frozen = root / "snapshot"
        if (frozen / "marker").read_bytes() != b"before\n" or hash_file(frozen / "blob") != expected_hash:
            raise ValueError("snapshot did not preserve known contents")
        try:
            (frozen / "forbidden").write_bytes(b"must fail")
        except OSError as error:
            if error.errno != 30:  # EROFS, even when the experiment runs as root.
                raise
        else:
            raise ValueError("read-only snapshot accepted a write")


def measure_image(image: Path, target: Path, filesystem: str, chunk: bytes, expected_hash: str) -> dict:
    """Measure durable creation, cold/warm stat and streaming read with checksums."""
    # Put the same content and sync boundaries on both filesystems.
    with mount_image(image, target, filesystem) as root:
        live = root / "live"
        if filesystem == "btrfs":
            run("btrfs", "subvolume", "create", str(live))
        else:
            live.mkdir()
        small = live / "small"
        small.mkdir()
        sync_directory(live)
        sync_directory(root)
        times = {"blob_write_fsync_s": write_durable(live / "blob", chunk, BLOB_MIB)}
        start = time.perf_counter()
        for number in range(FILE_COUNT):
            write_durable(small / f"{number:06d}", f"entry {number}\n".encode(), 1)
        times["small_create_fsync_s"] = time.perf_counter() - start

        # Stat fixed names to include directory lookup without collecting a walk.
        drop_caches()
        for temperature in ("cold", "warm"):
            start = time.perf_counter()
            for number in range(FILE_COUNT):
                (small / f"{number:06d}").stat()
            times[f"stat_{temperature}_s"] = time.perf_counter() - start

        # Hashing is deliberately included in the equal read workload.
        drop_caches()
        for temperature in ("cold", "warm"):
            start = time.perf_counter()
            if hash_file(live / "blob") != expected_hash:
                raise ValueError("read checksum mismatch")
            times[f"read_hash_{temperature}_s"] = time.perf_counter() - start

        # Check a native read-only snapshot before changing the live marker.
        write_durable(live / "marker", b"before\n", 1)
        if filesystem == "btrfs":
            run("btrfs", "subvolume", "snapshot", "-r", str(live), str(root / "snapshot"))
            sync_directory(root)
        with (live / "marker").open("wb", buffering=0) as output:
            output.write(b"after\n")
            os.fsync(output.fileno())
        os.sync()

    # Reopen the actual loop device and filesystem, not just Python file handles.
    with mount_image(image, target, filesystem) as root:
        verify_tree(root, expected_hash, filesystem == "btrfs")
    return {"filesystem": filesystem, "image": str(image), "seconds": times, "reopen_verified": True}


def main() -> None:
    """Run three alternating pairs or verify retained images without formatting."""
    # Restrict cache dropping and formatting to an explicit isolated-VM invocation.
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--isolated-vm", action="store_true", required=True)
    parser.add_argument("--reopen", type=Path)
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error("root is required for loop mounts and guest cache dropping")

    # Generate deterministic incompressible data outside measured operations.
    chunk = random.Random(20261101).randbytes(1024 * 1024)
    digest = hashlib.sha256()
    for _ in range(BLOB_MIB):
        digest.update(chunk)
    expected_hash = digest.hexdigest()

    # Reopen mode cannot format, replace or delete retained images.
    if args.reopen:
        directory = args.reopen.resolve(strict=True)
        if directory.parent != STORAGE_ROOT or not directory.name.startswith("compare-"):
            parser.error("reopen path must be a compare-* directory under the storage root")
        for index in range(6):
            filesystem = ("ext4", "btrfs")[index % 2]
            image = directory / f"{index // 2}-{filesystem}.img"
            if filesystem not in ("ext4", "btrfs") or not image.is_file() or image.is_symlink():
                raise ValueError(f"unexpected image: {image}")
            with mount_image(image, directory / "mount", filesystem) as root:
                verify_tree(root, expected_hash, filesystem == "btrfs")
            print(f"reopen verified: {image}", flush=True)
        return

    # New directories and exclusive image creation make accidental reformatting impossible.
    directory = Path(tempfile.mkdtemp(prefix="compare-", dir=STORAGE_ROOT))
    target = directory / "mount"
    target.mkdir()
    report = {"kernel": platform.release(), "image_bytes": IMAGE_BYTES, "blob_mib": BLOB_MIB,
              "small_file_count": FILE_COUNT, "blob_sha256": expected_hash,
              "outer_filesystem": run("findmnt", "-no", "FSTYPE,SOURCE,OPTIONS", "-T", str(directory)).strip(),
              "mkfs_ext4": run("mkfs.ext4", "-V").strip(), "mkfs_btrfs": run("mkfs.btrfs", "--version").strip(),
              "results": []}
    print(f"retained directory: {directory}", flush=True)
    for round_number in range(3):
        order = ("ext4", "btrfs") if round_number % 2 == 0 else ("btrfs", "ext4")
        for filesystem in order:
            image = directory / f"{round_number}-{filesystem}.img"
            create_image(image, filesystem)
            result = measure_image(image, target, filesystem, chunk, expected_hash)
            report["results"].append(result)
            (directory / "results.json").write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
            print(json.dumps(result), flush=True)
    print(f"results: {directory / 'results.json'}", flush=True)


if __name__ == "__main__":
    main()

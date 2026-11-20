"""Boot an image whose authorization no pinned signer made; require refusal.

The disk's data directory pins one signer, and the image's card carries an
authorization by another, as a card rewritten after first boot would. The
guest must print SPACEWAVE_OS_ERROR naming the unpinned signer, and must never
print that it authorized, froze or opened the image or created its data
Volume. A failed bootstrap pauses PID 1 forever, so the driver stops QEMU once
the error appears. It prints REFUSAL_TRIAL_PASS and exits 0 only if every check
passes. The guest's console streams to stdout, so a failure keeps it.
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

from kill_trial import Console

REFUSAL = r"SPACEWAVE_OS_ERROR: image authorization: signer \S+ is not pinned"
OPENED = ["SPACEWAVE_OS_AUTHORIZED", "SPACEWAVE_OS_FROZEN", "SPACEWAVE_OS_DATA_CREATED"]


def note(message: str) -> None:
    """Print a driver line beside the guest's console."""
    print(f"refusal-trial: {message}", flush=True)


def main() -> int:
    """Boot the given disk once and check that it refuses the image."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["kernel", "initramfs", "disk"]:
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--accel", choices=["kvm", "tcg"], default="kvm")
    arguments = parser.parse_args()
    if not os.access(arguments.disk, os.W_OK):
        parser.error(f"the disk must be writable: {arguments.disk}")

    # Wait for the refusal, then require that nothing past it ran.
    console = Console(arguments, shell=False)
    try:
        console.wait(REFUSAL, 240)
        with console.changed:
            opened = [m for m in OPENED if any(m in line for line in console.lines)]
        if opened:
            raise RuntimeError(f"the refused boot printed {opened}")
    except RuntimeError as err:
        note(f"FAILED: {err}")
        return 1
    finally:
        console.process.kill()
        console.process.wait()
        console.reader.join()
    note("REFUSAL_TRIAL_PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())

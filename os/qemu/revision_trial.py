"""Move a console guest to a staged revision, then roll back a broken one.

The disk holds the accepted image, a data Volume with the rehearsal's probe,
and under /staging two more image stores with their authorizations: good, a
revision the pinned key signed, and broken, one whose workload no longer
matches the boot set's init. The guest mounts its own disk a second time and
stages each revision beside the console's read-only reader, then reboots.
The driver requires four boots: the accepted image stages good; good boots on
trial, is accepted and stages broken; broken boots on trial, fails and
reboots; the next boot rolls broken back and serves good. Each boot that
reaches a shell must still read the probe. It prints REVISION_TRIAL_PASS and
exits 0 only if every check passes. The guest's console streams to stdout, so
a failure keeps it.
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

from kill_trial import Console, output

ROOT = r"root=(\S+)"


def note(message: str) -> None:
    """Print a driver line beside the guest's console."""
    print(f"revision-trial: {message}", flush=True)


def probe(console: Console) -> None:
    """Require the rehearsal's probe in the data Volume."""
    start = len(console.lines)
    console.send(
        'if [ "$(cat /data/rehearsal/probe)" = rehearsed ]; then echo PROBE_OK; else echo PROBE_LOST; fi'
    )
    if (
        not console.wait(output("(PROBE_OK|PROBE_LOST)"), 60, start)
        .rstrip()
        .endswith("PROBE_OK")
    ):
        raise RuntimeError("the data Volume lost the rehearsal probe")


def stage_and_reboot(console: Console, name: str) -> str:
    """Stage /staging/name as the trial, reboot, and return its root."""
    start = len(console.lines)
    console.send(
        f"mkdir /tmp/disk && mount -t btrfs /dev/vda /tmp/disk"
        f" && /workload stage /tmp/disk/image /tmp/disk/staging/{name}/image /tmp/disk/staging/{name}/authorization"
        f"; status=$?; umount /tmp/disk; echo STAGE_STATUS=$status"
    )
    if (
        not console.wait(output("STAGE_STATUS=[0-9]+"), 300, start)
        .rstrip()
        .endswith("=0")
    ):
        raise RuntimeError(f"staging {name} failed")
    staged = console.wait("SPACEWAVE_OS_STAGED " + ROOT, 0, start)
    console.send("reboot")
    stopped(console, "SPACEWAVE_OS_REBOOT")
    return re.search(ROOT, staged)[1]


def stopped(console: Console, marker: str) -> None:
    """Wait for QEMU to exit after the guest printed marker."""
    status = console.process.wait(timeout=300)
    console.reader.join()
    console.wait(marker, 0)
    if status != 0:
        raise RuntimeError(f"guest exited with status {status}")


def served(console: Console, start: int = 0) -> str:
    """Return the root the guest authorized and served."""
    line = console.wait("SPACEWAVE_OS_AUTHORIZED signer=\\S+ " + ROOT, 0, start)
    return re.search(ROOT, line)[1]


def main() -> int:
    """Run the four boots on the given disk."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["kernel", "initramfs", "disk"]:
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--accel", choices=["kvm", "tcg"], default="kvm")
    arguments = parser.parse_args()
    if not os.access(arguments.disk, os.W_OK):
        parser.error(f"the disk must be writable: {arguments.disk}")

    console: Console | None = None
    try:
        # Boot 1: the accepted image stages good.
        console = Console(arguments)
        accepted = served(console)
        probe(console)
        good = stage_and_reboot(console, "good")
        if good == accepted:
            raise RuntimeError("good names the accepted root")

        # Boot 2: good boots on trial, is accepted, and stages broken.
        console = Console(arguments)
        console.wait("SPACEWAVE_OS_TRIAL root=" + re.escape(good), 0)
        console.wait("SPACEWAVE_OS_ACCEPTED root=" + re.escape(good), 60)
        probe(console)
        broken = stage_and_reboot(console, "broken")
        note(f"good {good} accepted; broken {broken} staged")

        # Boot 3: broken boots on trial, fails before its shell, and reboots.
        console = Console(arguments, shell=False)
        console.wait("SPACEWAVE_OS_TRIAL root=" + re.escape(broken), 240)
        stopped(console, "SPACEWAVE_OS_REBOOT")
        if any("SPACEWAVE_OS_ACCEPTED" in line for line in console.lines):
            raise RuntimeError("the broken trial was accepted")

        # Boot 4: broken rolls back and good serves, with the probe intact.
        console = Console(arguments)
        console.wait("SPACEWAVE_OS_ROLLED_BACK root=" + re.escape(broken), 0)
        if served(console) != good:
            raise RuntimeError("the rollback did not serve the accepted revision")
        probe(console)
        console.poweroff()
    except (RuntimeError, OSError, subprocess.TimeoutExpired) as err:
        note(f"FAILED: {err}")
        if console is not None and console.process.poll() is None:
            console.process.kill()
        return 1
    note("REVISION_TRIAL_PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())

"""Move a console guest to a staged revision, then roll back a broken one.

The disk holds the accepted image, a data Volume with the rehearsal's probe,
and under /staging two more image stores with their authorizations: good, a
revision the pinned key signed, and broken, one whose workload no longer
matches the boot set's init. The guest mounts its own disk a second time and
stages each revision beside the console's read-only reader, then reboots.
The driver requires seven boots, two of them cut by SIGKILL to QEMU as a
power cut would: the accepted image starts staging good and is cut at its
first disk write; the next boot still serves the accepted image, stages good
and reboots; good boots on trial and is cut once marked attempted; the next
boot rolls good back, serves the accepted image and restages good, copying
no block; good boots on trial, is accepted and stages broken; broken boots on
trial, fails and reboots; the last boot rolls broken back and serves good.
Each boot that reaches a shell must still read the probe. It prints REVISION_TRIAL_PASS and
exits 0 only if every check passes. The guest's console streams to stdout, so
a failure keeps it.
"""

from __future__ import annotations

import argparse
import os
import re
import signal
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


def stage_command(name: str) -> str:
    """Return the guest command that stages /staging/name as the trial."""
    return (
        "mkdir /tmp/disk && mount -t btrfs /dev/vda /tmp/disk"
        f" && /workload stage /tmp/disk/image /tmp/disk/staging/{name}/image /tmp/disk/staging/{name}/authorization"
        "; status=$?; umount /tmp/disk; echo STAGE_STATUS=$status"
    )


def stage_and_reboot(console: Console, name: str) -> tuple[str, int]:
    """Stage /staging/name as the trial, reboot, and return its root and the
    number of blocks the staging copied."""
    start = len(console.lines)
    console.send(stage_command(name))
    if (
        not console.wait(output("STAGE_STATUS=[0-9]+"), 300, start)
        .rstrip()
        .endswith("=0")
    ):
        raise RuntimeError(f"staging {name} failed")
    staged = console.wait("SPACEWAVE_OS_STAGED " + ROOT, 0, start)
    copied = console.wait(r"SPACEWAVE_OS_STAGE_COPIED blocks=\d+", 0, start)
    console.send("reboot")
    stopped(console, "SPACEWAVE_OS_REBOOT")
    return re.search(ROOT, staged)[1], int(copied.rsplit("=", 1)[1])


def cut(console: Console, marker: str) -> None:
    """Cut the guest's power with SIGKILL to QEMU once marker appears."""
    console.wait(marker, 240)
    console.process.send_signal(signal.SIGKILL)
    console.process.wait()
    console.reader.join()
    note(f"SIGKILL qemu pid={console.process.pid} after {marker!r}")


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
    """Run the seven boots on the given disk."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["kernel", "initramfs", "disk"]:
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--accel", choices=["kvm", "tcg"], default="kvm")
    arguments = parser.parse_args()
    if not os.access(arguments.disk, os.W_OK):
        parser.error(f"the disk must be writable: {arguments.disk}")

    console: Console | None = None
    try:
        # Boot 1: the accepted image starts staging good; power is cut at
        # the staging's first disk write.
        console = Console(arguments)
        accepted = served(console)
        probe(console)
        console.send("echo STAGE_CUT_START && " + stage_command("good"))
        console.kill_mid_write(output("STAGE_CUT_START"))

        # Boot 2: the cut staging published nothing; the accepted image
        # serves, then stages good in full.
        console = Console(arguments)
        if any("SPACEWAVE_OS_TRIAL root=" in line for line in console.lines):
            raise RuntimeError("a cut staging published its trial")
        if served(console) != accepted:
            raise RuntimeError("the cut staging moved the accepted revision")
        probe(console)
        good, copied = stage_and_reboot(console, "good")
        if good == accepted or copied == 0:
            raise RuntimeError(f"good staged {copied} blocks under root {good}")

        # Boot 3: good boots on trial; power is cut once it is marked
        # attempted.
        console = Console(arguments, shell=False)
        cut(console, "SPACEWAVE_OS_TRIAL root=" + re.escape(good))

        # Boot 4: the attempted trial rolls back; the accepted image serves
        # and restages good, whose blocks the store already holds.
        console = Console(arguments)
        console.wait("SPACEWAVE_OS_ROLLED_BACK root=" + re.escape(good), 0)
        if served(console) != accepted:
            raise RuntimeError(
                "the cut trial did not roll back to the accepted revision"
            )
        probe(console)
        restaged, copied = stage_and_reboot(console, "good")
        if restaged != good or copied != 0:
            raise RuntimeError(f"restaging good copied {copied} blocks")

        # Boot 5: good boots on trial, is accepted, and stages broken.
        console = Console(arguments)
        console.wait("SPACEWAVE_OS_TRIAL root=" + re.escape(good), 0)
        console.wait("SPACEWAVE_OS_ACCEPTED root=" + re.escape(good), 60)
        probe(console)
        broken, _ = stage_and_reboot(console, "broken")
        note(f"good {good} accepted; broken {broken} staged")

        # Boot 6: broken boots on trial, fails before its shell, and reboots.
        console = Console(arguments, shell=False)
        console.wait("SPACEWAVE_OS_TRIAL root=" + re.escape(broken), 240)
        stopped(console, "SPACEWAVE_OS_REBOOT")
        if any("SPACEWAVE_OS_ACCEPTED" in line for line in console.lines):
            raise RuntimeError("the broken trial was accepted")

        # Boot 7: broken rolls back and good serves, with the probe intact.
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

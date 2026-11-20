"""Kill the console guest twice mid-write and check every acknowledged record.

The guest writes 16 records of 256 KiB, syncing each and acknowledging it with
its SHA-256, then starts a 256 MiB write. At the first host disk write after
that start, the driver kills QEMU with SIGKILL. It reboots, checks every
acknowledged record, repeats the write and the kill, and checks all 32 records
before a clean poweroff. It prints KILL_TRIAL_PASS and exits 0 only if every
check passes. The guest's console streams to stdout, so a failure keeps it.
"""

from __future__ import annotations

import argparse
import os
import re
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

from run_app import qemu_command

RECORDS = 16
ACK = re.compile(r"ACK 262144 ([0-9a-f]{64})  (/data/[a-z-]+/record-[0-9]+)")


def output(marker: str) -> str:
    """Return a pattern for marker printed alone, after any shell prompts.

    With echo off, a command's output follows the prompts on the same line,
    as in "spacewave# > > spacewave# ACK_OK". Echoed input never matches,
    since it ends with the command that prints the marker.
    """
    return rf"(?:^|[#>] ){marker}$"


def note(message: str) -> None:
    """Print a driver line beside the guest's console."""
    print(f"kill-trial: {message}", flush=True)


class Console:
    """One booted console guest whose serial lines the driver waits on."""

    def __init__(self, arguments: argparse.Namespace) -> None:
        # Start QEMU and stream its console to stdout and to the line list.
        command = qemu_command(arguments.accel, arguments.kernel, arguments.initramfs, arguments.disk)
        self.process = subprocess.Popen(
            command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
        self.lines: list[str] = []
        self.changed = threading.Condition()
        self.reader = threading.Thread(target=self.read, daemon=True)
        self.reader.start()

        # Wait for the shell, then turn echo off so typed text never matches.
        self.wait("SPACEWAVE_OS_CONSOLE_READY pid=", 240)
        self.send("stty -echo && echo SPACEWAVE_OS_INPUT_PRIVATE")
        self.wait(output("SPACEWAVE_OS_INPUT_PRIVATE"), 30)

    def read(self) -> None:
        """Copy console lines to stdout and wake waiters on each."""
        assert self.process.stdout is not None
        for line in self.process.stdout:
            print(line, end="", flush=True)
            with self.changed:
                self.lines.append(line.replace("\r", ""))
                self.changed.notify_all()
        with self.changed:
            self.changed.notify_all()

    def send(self, text: str) -> None:
        """Type text into the guest shell."""
        assert self.process.stdin is not None
        self.process.stdin.write(text + "\n")
        self.process.stdin.flush()

    def wait(self, pattern: str, seconds: float, start: int = 0) -> str:
        """Return the first line from start on that holds pattern."""
        deadline = time.monotonic() + seconds
        with self.changed:
            while True:
                for line in self.lines[start:]:
                    if re.search(pattern, line):
                        return line
                left = deadline - time.monotonic()
                if left <= 0 or not self.reader.is_alive():
                    raise RuntimeError(f"no console line matching {pattern!r}")
                self.changed.wait(left)

    def kill_mid_write(self, marker: str) -> None:
        """Kill QEMU at its first disk write after the marker line."""
        self.wait(marker, 600)
        io = Path(f"/proc/{self.process.pid}/io")
        start, deadline = written(io), time.monotonic() + 60
        while (seen := written(io)) <= start:
            if time.monotonic() > deadline:
                raise RuntimeError(f"no disk write after {marker!r}")
            time.sleep(0.01)
        self.process.send_signal(signal.SIGKILL)
        self.process.wait()
        self.reader.join()
        note(f"SIGKILL qemu pid={self.process.pid} write_bytes start={start} seen={seen}")

    def acknowledged(self) -> list[str]:
        """Return the records the guest acknowledged, in sha256sum -c form."""
        with self.changed:
            return [f"{m[1]}  {m[2]}" for line in self.lines if (m := ACK.search(line))]

    def poweroff(self) -> None:
        """Stop the guest and require its clean storage close."""
        self.send("sync; poweroff")
        status = self.process.wait(timeout=120)
        self.reader.join()
        self.wait(output("SPACEWAVE_OS_PASS"), 0)
        if status != 0:
            raise RuntimeError(f"guest exited with status {status}")


def written(io: Path) -> int:
    """Return the process's write_bytes from its /proc io file."""
    for line in io.read_text().splitlines():
        if line.startswith("write_bytes:"):
            return int(line.split()[1])
    raise RuntimeError(f"{io} has no write_bytes")


def writer(directory: str, marker: str) -> str:
    """Return the guest script that writes and acknowledges the records."""
    return f"""cat > /tmp/w.sh <<'EOF'
set -eu
mkdir /data/{directory}
sync /data
for i in $(seq 1 {RECORDS}); do
  path=/data/{directory}/record-$i
  dd if=/dev/urandom of=/tmp/payload bs=262144 count=1 2>/dev/null
  hash=$(sha256sum /tmp/payload)
  hash=${{hash%% *}}
  cat /tmp/payload > "$path"
  sync "$path"
  sync /data/{directory}
  echo "ACK 262144 $hash  $path"
done
dd if=/dev/urandom of=/data/{directory}/inflight bs=1048576 count=2 2>/dev/null
echo {marker}
exec dd if=/dev/urandom of=/data/{directory}/inflight bs=1048576 count=256 seek=2 conv=notrunc
EOF
sh /tmp/w.sh"""


def checker(acks: list[str], directories: list[str]) -> str:
    """Return the guest script that checks every acknowledged record."""
    records = " ".join(f"/data/{d}/record-*" for d in directories)
    listed = "\n".join(acks)
    count = len(acks)
    # ash ignores set -e inside a list such as "( ... ) || echo", so each test
    # exits the subshell itself and only a full match prints ACK_OK.
    return f"""cat > /tmp/acknowledged <<'ACKS'
{listed}
ACKS
if (sha256sum -c /tmp/acknowledged >/dev/null || exit 1; for p in {records}; do test "$(stat -c %s "$p")" = 262144 || exit 1; done; test $(ls {records} | wc -l) = {count}); then echo ACK_OK; else echo CHECK_FAILED; fi"""


def check(console: Console, acks: list[str], directories: list[str]) -> None:
    """Check the records after a reopen, or raise."""
    start = len(console.lines)
    console.send(checker(acks, directories))
    if not console.wait(output("(ACK_OK|CHECK_FAILED)"), 120, start).rstrip().endswith("ACK_OK"):
        raise RuntimeError(f"acknowledged records lost in {directories}")
    note(f"{len(acks)} acknowledged records intact in {directories}")


def acknowledge(console: Console, directory: str) -> list[str]:
    """Return the trial's acknowledged records, requiring all of them."""
    acks = console.acknowledged()
    if len(acks) != RECORDS:
        raise RuntimeError(f"{directory}: guest acknowledged {len(acks)} of {RECORDS} records")
    return acks


def main() -> int:
    """Run both kill trials and the final check on the given disk."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["kernel", "initramfs", "disk"]:
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--accel", choices=["kvm", "tcg"], default="kvm")
    arguments = parser.parse_args()
    if not os.access(arguments.disk, os.W_OK):
        parser.error(f"the disk must be writable: {arguments.disk}")

    console: Console | None = None
    try:
        # Trial 1: acknowledged records, then SIGKILL during a further write.
        console = Console(arguments)
        console.send(writer("records", "FINAL_CRASH_ONE_START"))
        console.kill_mid_write(output("FINAL_CRASH_ONE_START"))
        first = acknowledge(console, "records")

        # Reopen: check trial 1, then trial 2.
        console = Console(arguments)
        check(console, first, ["records"])
        console.send(writer("input-records", "FINAL_CRASH_TWO_START"))
        console.kill_mid_write(output("FINAL_CRASH_TWO_START"))
        second = acknowledge(console, "input-records")

        # Reopen: check all records, then a clean poweroff.
        console = Console(arguments)
        check(console, first + second, ["records", "input-records"])
        console.poweroff()
    except (RuntimeError, OSError, subprocess.TimeoutExpired) as err:
        note(f"FAILED: {err}")
        if console is not None and console.process.poll() is None:
            console.process.kill()
        return 1
    note("KILL_TRIAL_PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())

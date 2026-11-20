"""Run one QEMU guest inside a Forge-owned container and join its clean stop."""

from __future__ import annotations

import argparse
import signal
import subprocess
import sys
from pathlib import Path
from types import FrameType
from typing import IO


def qemu_command(
    accel: str,
    kernel: Path,
    initramfs: Path,
    disk: Path,
    config_disk: Path | None = None,
    port: int | None = None,
    dhcp: bool = False,
) -> list[str]:
    """Return the console guest's QEMU command, its serial console on stdio.

    With dhcp, the kernel leases an address from QEMU's user network before
    init, which binds the lease's resolvers as the image's resolv.conf.
    """
    append = "console=ttyS0,115200 rdinit=/init panic=-1 quiet GOMAXPROCS=1 SPACEWAVE_OS_MODE=console SPACEWAVE_OS_STORE=/store/image SPACEWAVE_OS_DATA=/store/data SPACEWAVE_OS_TTY=/dev/ttyS0"
    if config_disk is not None:
        append += " SPACEWAVE_OS_CONFIG=/config-store/data"
    if dhcp:
        append += " ip=dhcp"
    command = [
        "qemu-system-x86_64",
        "-accel",
        accel,
        "-cpu",
        "host" if accel == "kvm" else "max",
        "-m",
        "1536",
        "-smp",
        "1",
        "-kernel",
        str(kernel),
        "-initrd",
        str(initramfs),
        "-append",
        append,
        "-drive",
        f"file={disk},if=virtio,format=raw,cache=none",
    ]
    if config_disk is not None:
        command += ["-drive", f"file={config_disk},if=virtio,format=raw,cache=none"]
    if port is not None or dhcp:
        forward = "" if port is None else f",hostfwd=tcp:127.0.0.1:{port}-:3000"
        command += [
            "-netdev",
            f"user,id=net{forward}",
            "-device",
            "virtio-net-pci,netdev=net",
        ]
    else:
        command += ["-nic", "none"]
    return command + [
        "-display",
        "none",
        "-monitor",
        "none",
        "-serial",
        "stdio",
        "-no-reboot",
    ]


class Guest:
    """Keep the console command stream and QEMU process in one container lifetime."""

    def __init__(self, arguments: argparse.Namespace) -> None:
        self.arguments = arguments
        self.startup = arguments.startup.read_text()
        self.process: subprocess.Popen[str] | None = None
        self.private_console = False
        self.stopping = False
        self.shutdown_sent = False
        self.clean = False
        self.application_started = False
        self.application_clean = False

    def send(self, command: str) -> None:
        """Write console input without exposing application configuration in logs."""
        assert self.process is not None and self.process.stdin is not None
        try:
            self.process.stdin.write(command + "\n")
            self.process.stdin.flush()
        except BrokenPipeError:
            pass

    def stop(self, _signal: int, _frame: FrameType | None) -> None:
        """Request guest cleanup; Docker retains custody until this process exits."""
        self.stopping = True
        if self.private_console and not self.shutdown_sent:
            self.shutdown_sent = True
            self.send(
                'if [ -n "$GITEA_JOB" ]; then kill -TERM -$GITEA_JOB; wait $GITEA_JOB; app_status=$?; echo SPACEWAVE_OS_APP_STOPPED status=$app_status; fi; echo SPACEWAVE_OS_CONFIG_CLOSED; sha256sum /data/config/app.ini; sync; poweroff'
            )

    def read_console(self, output: IO[str]) -> None:
        """Stream guest output, reacting to actual console and shutdown events."""
        for line in output:
            print(line, end="", flush=True)
            if "SPACEWAVE_OS_CONSOLE_READY pid=" in line:
                self.send("stty -echo && echo SPACEWAVE_OS_INPUT_PRIVATE")
            if line.strip() == "SPACEWAVE_OS_INPUT_PRIVATE":
                self.private_console = True
                if self.stopping:
                    self.stop(signal.SIGTERM, None)
                else:
                    self.send(self.startup)
            if "SPACEWAVE_OS_APP_STARTED pid=" in line:
                self.application_started = True
            if line.strip() == "SPACEWAVE_OS_APP_STOPPED status=0":
                self.application_clean = True
            if line.strip() == "SPACEWAVE_OS_PASS":
                self.clean = True

    def run(self) -> int:
        """Start the declared VM, then let its console and process report completion."""
        arguments = self.arguments
        command = qemu_command(
            arguments.accel,
            arguments.kernel,
            arguments.initramfs,
            arguments.disk,
            arguments.config_disk,
            arguments.port,
            arguments.dhcp,
        )
        signal.signal(signal.SIGTERM, self.stop)
        signal.signal(signal.SIGINT, self.stop)
        with subprocess.Popen(
            command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        ) as process:
            self.process = process
            assert process.stdout is not None
            self.read_console(process.stdout)
            status = process.wait()
        if (
            status != 0
            or not self.clean
            or self.application_started
            and not self.application_clean
        ):
            print(
                f"Guest stopped without clean storage confirmation: status={status}",
                file=sys.stderr,
            )
            return 1
        if arguments.receipt is not None:
            arguments.receipt.write_text("SPACEWAVE_OS_PASS\n")
        return 0


def main() -> int:
    """Consume existing explicit bindings; fixture creation belongs to the caller."""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["kernel", "initramfs", "disk", "startup"]:
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument(
        "--config-disk", type=Path, help="second store, at /config-store"
    )
    parser.add_argument("--receipt", type=Path)
    parser.add_argument(
        "--port", type=int, help="host port forwarded to guest port 3000"
    )
    parser.add_argument(
        "--dhcp", action="store_true", help="user network leased by ip=dhcp"
    )
    parser.add_argument("--accel", choices=["kvm", "tcg"], default="kvm")
    return Guest(parser.parse_args()).run()


if __name__ == "__main__":
    sys.exit(main())

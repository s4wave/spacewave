"""Write the image archive that makes a Device run and enroll its daemon.

The archive holds a static spacewave binary at /usr/bin/spacewave, the CA
bundle its TLS dials verify against, and the empty mountpoints spacewave-init
binds at boot: /etc/resolv.conf (the kernel's DHCP lease), /etc/spacewave/grant
(the card's bootstrap grant) and /var/lib/spacewave (the daemon's state).
"""

from __future__ import annotations

import argparse
import io
import subprocess
import tarfile
from pathlib import Path


def require_static(binary: Path) -> None:
    """Refuse a dynamic binary: the image carries no loader or libraries."""
    headers = subprocess.run(
        ["readelf", "-lW", str(binary)], capture_output=True, text=True, check=True
    ).stdout
    if "INTERP" in headers or "DYNAMIC" in headers:
        raise ValueError(f"{binary} is dynamic; build it with CGO_ENABLED=0")


def entry(name: str, mode: int, kind: bytes = tarfile.REGTYPE, size: int = 0) -> tarfile.TarInfo:
    """Return a root-owned archive entry with a fixed time."""
    info = tarfile.TarInfo(name)
    info.type, info.mode, info.size = kind, mode, size
    info.uid = info.gid = info.mtime = 0
    return info


def package_daemon(spacewave: Path, certificates: Path, output: Path) -> None:
    """Stream the binary, the bundle and the mountpoints into a new archive."""
    require_static(spacewave)
    with tarfile.open(output, "x") as archive:
        for name, source, mode in [
            ("usr/bin/spacewave", spacewave, 0o755),
            ("etc/ssl/certs/ca-certificates.crt", certificates, 0o644),
        ]:
            with source.open("rb") as content:
                archive.addfile(entry(name, mode, size=source.stat().st_size), content)
        hosts = b"127.0.0.1 localhost\n"
        archive.addfile(entry("etc/hosts", 0o644, size=len(hosts)), io.BytesIO(hosts))
        for name in ["etc/resolv.conf", "etc/spacewave/grant"]:
            archive.addfile(entry(name, 0o444), io.BytesIO())
        archive.addfile(entry("var/lib/spacewave", 0o700, tarfile.DIRTYPE))


def main() -> None:
    """Require explicit inputs and a new archive path."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("spacewave", type=Path)
    parser.add_argument("certificates", type=Path)
    parser.add_argument("output", type=Path)
    arguments = parser.parse_args()
    package_daemon(arguments.spacewave, arguments.certificates, arguments.output)


if __name__ == "__main__":
    main()

"""Stream the explicit Gitea and Git runtime into an immutable-image archive."""

from __future__ import annotations

import argparse
import hashlib
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path


def copy_program(source: Path, destination: Path, root: Path) -> None:
    """Copy one executable and its loader dependencies, preserving guest paths."""
    target = root / str(destination).lstrip("/")
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)
    target.chmod(0o755)
    result = subprocess.run(
        ["ldd", str(source)], capture_output=True, text=True, check=False
    )
    for name in re.findall(r"(?:^|\s)(/[^\s]+)", result.stdout):
        library = Path(name)
        if not library.is_file():
            continue
        target = root / name.lstrip("/")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(library, target)
        target.chmod(0o755)


def package_apps(gitea: Path, output: Path) -> None:
    """Build regular archive entries from tools inside the selected build image."""
    with gitea.open("rb") as gitea_file:
        digest = hashlib.file_digest(gitea_file, "sha256").hexdigest()
    if digest != "1c86b689cd44f843299cb12b5b3eb1189d0638d5ffda04eb6b4dd0fe395422fc":
        raise ValueError(
            "Gitea input does not match the pinned 1.25.5 linux-amd64 binary"
        )
    with tempfile.TemporaryDirectory(prefix="spacewave-apps-") as directory:
        root = Path(directory)
        copy_program(gitea, Path("/bin/gitea"), root)
        for filename in ["/usr/bin/git", "/usr/bin/bash", "/usr/bin/env"]:
            copy_program(Path(filename), Path(filename), root)
        for name in ["git-http-backend", "git-upload-pack", "git-receive-pack"]:
            executable = Path("/usr/lib/git-core") / name
            copy_program(executable, executable, root)
        config = root / "etc"
        config.mkdir(exist_ok=True)
        (config / "passwd").write_text(
            "root:x:0:0:root:/data:/bin/sh\ngit:x:1000:1000:Git:/data/config/gitea:/bin/sh\n"
        )
        (config / "group").write_text("root:x:0:\ngit:x:1000:\n")
        (config / "nsswitch.conf").write_text(
            "passwd: files\ngroup: files\nhosts: files dns\n"
        )
        (config / "hosts").write_text("127.0.0.1 localhost\n")
        with tarfile.open(output, "x") as archive:
            for directory, folders, files in os.walk(root):
                folders.sort()
                for name in sorted(files):
                    source = Path(directory) / name
                    info = archive.gettarinfo(
                        str(source), str(source.relative_to(root))
                    )
                    info.uid = info.gid = info.mtime = 0
                    info.uname = info.gname = ""
                    with source.open("rb") as content:
                        archive.addfile(info, content)


def main() -> None:
    """Require explicit input and a new archive path; never alter a saved image."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("gitea", type=Path)
    parser.add_argument("output", type=Path)
    arguments = parser.parse_args()
    package_apps(arguments.gitea, arguments.output)


if __name__ == "__main__":
    main()

"""Verify excluded files and a read-only rerun against the authorized test copy."""

import hashlib
import json
import re
import subprocess
from pathlib import Path


def metadata(root):
    """Record native database and sidecar identities without reading their contents."""
    return {
        str(path.relative_to(root)): [path.stat().st_size, path.stat().st_mtime_ns]
        for path in sorted(root.rglob("*"))
        if path.is_file() and ".s4wave" in path.name
    }


def digest(path):
    """Hash one excluded file with a bounded read buffer."""
    result = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(8 * 1024 * 1024):
            result.update(chunk)
    return result.hexdigest()


def main():
    """Check byte preservation, then rerun the installed tool without modifying state."""
    root = Path("/Users/cjs/.spacewave.migrate-test")
    artifacts = Path(__file__).resolve().parent
    report_path = artifacts / "verification.json"
    report = json.loads(report_path.read_text())
    after = metadata(root)
    changed = sorted(
        name for name in set(after) | set(report["before_metadata"])
        if after.get(name) != report["before_metadata"].get(name)
    )
    assert all(name.startswith("p_local_") for name in changed), changed
    digests = {
        name: digest(root / name)
        for name in report["excluded_sha256_before"]
    }
    assert digests == report["excluded_sha256_before"], "excluded files changed"

    before_dry_run = metadata(root)
    result = subprocess.run(
        ["/Users/cjs/company/.tmp/sobject-migrate-once", "--state-path", str(root)],
        capture_output=True, text=True, check=True,
    )
    assert metadata(root) == before_dry_run, "dry run changed a database or sidecar"
    assert result.stdout.count("action=already-valid") == 12, "local validation incomplete"
    assert "complete eligible_repairs=0 dry_run=true" in result.stdout
    safe_output = re.sub(r'space="[^"]+"', 'space="<catalog name omitted>"', result.stdout)
    (artifacts / "dry-after.txt").write_text(safe_output)
    report.update(
        after_metadata=after,
        changed_files=changed,
        excluded_sha256_after=digests,
        excluded_files_unchanged=True,
        dry_run_metadata_unchanged=True,
        local_objects_valid=12,
    )
    report_path.write_text(json.dumps(report, indent=2) + "\n")
    print("PASS: 12 local objects valid; 24 excluded files byte-identical; dry-run database and sidecar metadata unchanged.")
    print("Changed files:", ", ".join(changed))


if __name__ == "__main__":
    main()

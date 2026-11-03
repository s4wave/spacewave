"""Run one bounded interval-model check or its expected failing mutation."""

import argparse
import hashlib
import shutil
import subprocess
from pathlib import Path


def mutate_source(source: str, case: str) -> str:
    """Remove one owning rule, refusing an absent or ambiguous source anchor."""
    if case == "ordinary":
        before = " + ordinaryKids(p)"
        after = ""
    elif case == "historical":
        before = "StartApply ==\n  /\\ ~walk.on\n"
        after = (
            before
            + "  /\\ (~Shadow \\/ (UserLive \\cup (Reach(cur) \\ kept)\n"
            + "       \\cup UNION {Reach(r) : r \\in staged \\cup PartRoots})"
            + " \\ stored = {})\n"
        )
    else:
        # StrayRule is already an explicit production/mutation switch.
        return source
    if source.count(before) != 1:
        raise ValueError(f"expected exactly one {case} mutation anchor")
    return source.replace(before, after, 1)


def main() -> int:
    """Copy the exact model, apply the selected omission, and verify TLC's result."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--case", choices=("ordinary", "historical", "stray"), required=True
    )
    parser.add_argument("--mutant", action="store_true")
    parser.add_argument("--jar", type=Path, required=True)
    parser.add_argument("--java", default="java")
    parser.add_argument(
        "--output", type=Path, required=True, help="new evidence directory"
    )
    parser.add_argument("--timeout", type=int, default=60)
    args = parser.parse_args()

    # Keep every input and the complete trace beside its bounded run. Never
    # overwrite an earlier run or mutate the repository's proof input.
    fixtures = Path(__file__).resolve().parent
    repo = fixtures.parent.parent
    suffixes = {
        "ordinary": "missing-outside",
        "historical": "old-gate",
        "stray": "ignore",
    }
    suffix = f"-{suffixes[args.case]}" if args.mutant else ""
    config = f"IntervalCounts-{args.case}{suffix}.cfg"
    config_text = (fixtures / config).read_text(encoding="utf-8")
    if args.mutant and args.case == "historical":
        properties = [
            line.strip()
            for line in config_text.splitlines()
            if line.startswith("PROPERTY")
        ]
        if properties != ["PROPERTY RecordsRetire"]:
            raise ValueError("historical mutation must check only RecordsRetire")
    original = (repo / "IntervalCounts.tla").read_text(encoding="utf-8")
    source = mutate_source(original, args.case) if args.mutant else original
    args.output.mkdir(parents=True, exist_ok=False)
    output = args.output.resolve()
    (output / "IntervalCounts.tla").write_text(source, encoding="utf-8")
    for filename in ("IntervalCountsChecks.tla", config):
        shutil.copyfile(fixtures / filename, output / filename)
    (output / "inputs.sha256").write_text(
        f"{hashlib.sha256(original.encode()).hexdigest()}  original IntervalCounts.tla\n"
        f"{hashlib.sha256(source.encode()).hexdigest()}  checked IntervalCounts.tla\n"
        f"{hashlib.sha256((fixtures / config).read_bytes()).hexdigest()}  {config}\n",
        encoding="utf-8",
    )

    # These directed checks are small; full safety uses the separate unchanged
    # three-commit configuration. A timeout is incomplete, never a passed check.
    command = [
        args.java,
        "-XX:ActiveProcessorCount=1",
        "-Xmx512m",
        "-XX:+UseSerialGC",
        "-jar",
        str(args.jar.resolve()),
        "-workers",
        "1",
        "-metadir",
        "states",
        "-config",
        config,
        "IntervalCountsChecks.tla",
    ]
    log_path = output / "result.log"
    try:
        with log_path.open("w", encoding="utf-8") as log:
            result = subprocess.run(
                command,
                cwd=output,
                stdout=log,
                stderr=subprocess.STDOUT,
                timeout=args.timeout,
                check=False,
            )
    except subprocess.TimeoutExpired:
        print(f"INCOMPLETE: timeout; {log_path}")
        return 124

    # An arbitrary parse/override failure is not mutation sensitivity. Require
    # the owning invariant or the sole temporal property's actual violation.
    text = log_path.read_text(encoding="utf-8")
    expected = {
        "ordinary": "Invariant CountsCover is violated",
        "historical": "Temporal properties were violated",
        "stray": "Invariant NoLeak is violated",
    }[args.case]
    if args.mutant:
        temporal_named = "Temporal property RecordsRetire was violated" in text
        passed = result.returncode != 0 and (
            expected in text or (args.case == "historical" and temporal_named)
        )
    else:
        passed = result.returncode == 0
    print(
        f"{'PASS' if passed else 'FAIL'}: {args.case}{suffix}; TLC exit={result.returncode}; {log_path}"
    )
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())

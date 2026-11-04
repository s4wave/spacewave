These checks use the exact parent `IntervalCounts.tla`. The runner copies it
and the selected fixture into a new evidence directory, checks one bounded
run through the repository's `tla-ts`, and preserves the inputs and full
trace. Run `bun install` first so `node_modules/.bin/tla-ts` exists.

From the repository root:

```sh
uv run --no-project --python 3.11 db/block/gc/testdata/interval-counts/check_interval_counts.py \
  --case ordinary --output /path/to/new-evidence
```

Repeat with `--case historical` and `--case stray`. Add `--mutant` to each
command to check sensitivity. The ordinary omission must violate
`CountsCover`; the historical admission gate must violate `RecordsRetire`;
the stray omission must violate `NoLeak` in the separate counted-free branch.
A setup error or timeout fails the check. The historical positive also requires
that its guided prefix reaches the missing-metadata case, so retirement cannot
pass merely because no record was created.

The unchanged three-commit discard obligation is
`IntervalCounts-discard-full3.cfg` (SHA-256
`f65e1a88d871f4d09fb89f00b09d54991240a9caf696e69ac42d309563aaeaa4`).
Copy it and the parent `IntervalCounts.tla` into an evidence directory, then
run there:

```sh
/path/to/repo/node_modules/.bin/tla-ts IntervalCounts.tla \
  -config IntervalCounts-discard-full3.cfg -workers 1
```

This directory is excluded from broad positive-model discovery. Its expected
failures and manual full proof remain explicit obligations; the directed
checks do not replace the full proof or change the canonical configurations.

`IntervalCounts-progress.cfg` is an additional unconstrained two-commit
`ProgressSpec` / `RecordsRetire` liveness obligation. Its full model check has
not completed. Run it by the same copy-and-check method as the manual full
safety proof. The checked historical run-through is a separate directed
retirement result and does not establish this full liveness property.

A source-backed ranking argument supports retirement within this finite model:
foreground commits, checkpoints, preparation, role changes, alias acquisition
and receipt acknowledgements have finite productive steps. With no repeated
external reset, each invalidated plan therefore eventually has a stable read.
Fair preparation and carrying remove waiting records. A valid read walks at
most the finite page set, then applies or takes bounded recovery; an invalid
read retries or skips all history. Neither skip-history retirement nor recovery
requires historical holes to become readable. Fair ordinary erasure removes
finite unowned hole sources; a retained hole keeps trust disabled and still
permits invalid-record retirement. Due records leave in finite sequence order.
This is an argument about the model's abstraction and fairness assumptions,
not a completed model check of the unconstrained progress selector.

The full safety configuration and invariants are unchanged. This run keeps no
checkpoint; a lost verifier starts again from the initial state.

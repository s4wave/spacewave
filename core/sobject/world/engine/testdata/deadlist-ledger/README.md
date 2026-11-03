The configurations in this directory are manual preparation-ledger checks.
The omission configurations exercise expected failures and are excluded from
broad positive-model discovery.

`DeadlistLedger-two.cfg` is the full two-record/three-save safety obligation,
including two pending preparations. Its SHA-256 is
`bef112cc5c1c0765f6555d8469d9472db56b4346c8d6c940e9ce27efa0e303a6`.
The ordinary parent configuration checks the smaller one-record bound; that
result does not replace this larger obligation.

Copy the current parent `DeadlistLedger.tla` and this configuration into a fresh
evidence directory, then run there with TLC 2.19 and Java 17:

```sh
java -XX:ActiveProcessorCount=2 -Xmx2g -XX:+UseSerialGC \
  -jar /path/to/tla2tools.jar -workers 2 -checkpoint 0 \
  -fp 56 -seed 3004680919690233610 -metadir states \
  -config DeadlistLedger-two.cfg DeadlistLedger.tla
```

Keep the full inputs and output. All invariants remain enabled. Periodic
checkpoints are disabled because the established verifier jar stalled while
coordinating a checkpoint; this safety-only configuration has no temporal
property requiring that coordination. A host interruption may require a fresh
run. A partial exploration is not a completed proof.

Before recovering any other run, freeze a complete, self-contained copy of
its checkpoint and queue pool files. Advancing a recovered TLC queue can delete
files referenced by its input checkpoint; preserving only the small checkpoint
headers does not preserve another restart.

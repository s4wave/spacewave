The parent `BackfillSnapshot.tla` models one nonempty startup World, one changed
World, and a later admission that leaves the World root and seqno unchanged.
It models the existing user-head read pin and controller-close join. Retained-set
lifetime and notifications are separate contracts and are not proved here.

Run the parent `BackfillSnapshot.cfg` as a normal positive model. To check one
omission, copy the model and that selector to a new evidence directory and run
TLC there with `-config <selector>.cfg BackfillSnapshot.tla`.

Expected failures are specific invariants, not setup errors or timeouts:

- `retained.cfg`: `CoherentHead`, using background ownership as the user head.
- `no-durable-wait.cfg`: `CoversDrainedTarget`, copying before a prior admission
  reaches the durable read head.
- `early-release.cfg`: `CopyReadable`, releasing the captured pin before copying.
- `untracked.cfg`: `SnapshotLifetime`, allowing teardown to close an active read.

The `available` set abstracts a locally readable World graph through the page
view: kept pages protected by the existing head pin plus its physical-store
frontier. It does not assert that a kept World root is physically stored or that
peer-only blocks can never be missing. `Sweep` models loss of an old graph after
its last live head/read owner ends; normal copy holes are a separate result.

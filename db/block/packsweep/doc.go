// Package packsweep proves which packs of a pack store hold no live block, and
// carries a sweep of them through stages a wait separates, so a pack store
// whose collector is off loses its dead packs without losing a reached block.
//
// A cycle runs as numbered steps. Each step writes its result to the scratch
// store before the next starts, and repeating a finished step does nothing:
//
//   - Snapshot lists the packs registered at time S and fills the pack catalog:
//     each pack read once, whole, with its keys and each key's refs. Packs
//     never change, so the catalog is kept across cycles and a later cycle
//     reads only the packs registered since.
//   - Walk follows the refs from the roots through the catalog into the live
//     set L, asking a RefSource for blocks no catalogued pack holds, and lists
//     the holes by packcopy's reasons.
//   - Judge reports each snapshot pack as dead, low liveness, young or kept.
//
// Invariants:
//   - A catalog pack row commits in the transaction that writes every key row
//     of the pack, so a pack row names a pack whose keys and refs are all in
//     the catalog.
//   - A key's refs row holds the refs of a copy whose bytes matched its hash,
//     and is replaced only by a better state: known refs over unknown refs
//     over corrupt.
//   - A key enters the frontier and the visited set in one transaction, and
//     leaves the frontier only in the transaction that adds its children, so
//     a walk killed at any point resumes to the same live set.
//   - A pack is judged dead only when the walk finished with no frontier left
//     and none of the pack's keys is in L or W.
//
// The later steps quarantine the dead packs, recheck them, move the passed
// ones to the store's trash, have the backend remove them, and purge them
// once removed for a wait. The invariants that keep them from losing a
// reached block:
//   - A pack the sweep trashes may hold the only copy of every block in it:
//     the write-back's bounded eviction drops a block's local copy once its
//     pack verifies in the bucket. So the sweep trashes only what the root
//     walk and the log prove unreferenced, and no local copy is a fallback.
//     The eviction confirms each block with the store's GetBlockExistsBatch,
//     which answers from registered packs alone, so a block whose only
//     bucket copy is in a held pack keeps its local copy. The write-back's
//     upload dedupe and ClearLocated decide block by block in the same way:
//     the store's LocateBlocks skips the rows that name a held pack.
//   - A pack object under the store's prefix leaves the store's listing only
//     when two things hold. First, no key in its tail index is reached from
//     the roots the backend lists or from a root the write log names. The
//     backend lists every root its store holds, by walking the store's own
//     owner nodes rather than a list of root kinds, with every restore point
//     the operator can return the store to; a root it cannot read fails the
//     listing, and no pack moves. Second, since its
//     quarantine began, no h row named the pack's number in a lookup that
//     served a read, a Has or a write's dedupe.
//   - A pack named by the pack index of any restore point is referenced
//     until no listed restore point names it, since a restore point must
//     rebuild from exactly the packs it names. The backend's listing reads
//     every version of every restore point, and fails while any of them
//     carries a pack index, so neither the sweep nor the compaction, whose
//     victims come from the same judged walk, moves a pack then; the purge
//     lists the roots again before it deletes.
//   - The backend lists roots only from a store that records them in its
//     ownership graph; a store that does not fails the listing.
//   - A quarantined pack is invisible to the index's Locate and extent
//     lookups, so a write that would dedupe against it stores the block again
//     in a new pack. A read that finds the block only there lifts the pack, or
//     records the miss that restores a trashing one.
//   - Every write of the backend's store is in the write log before it is
//     stored, deduped or pushed: a fill store's diverted write, an atomic
//     publication, and a reader's pin, which the log names as a root. A block
//     written after the roots were read is therefore in W.
//   - A write, a read operation or a stage open on the backend is in its
//     record of open operations from before it touches the store until it
//     ends, and the recheck passes no pack while one opened before the pack's
//     quarantine is open.
//   - A block in the write-back's durable queue is in no pack of the store,
//     so no step touches it.
//   - Where a pack's state is unsure (a log gap, a key with unknown refs, a
//     hole the walk could not read and the operator did not accept), the pack
//     stays.
//   - A pass is not final. The trash, before the backend marks a pack
//     trashing and before it moves a pack's objects, and the removal, before
//     the backend drops a pack, each read the packs' states, then pull the log
//     to its open end and walk W, and act only on packs none of whose keys is
//     in L or W then. A pack a root reached since its recheck is lifted, or
//     left trashing and marked for the restore.
//   - A restore records a pack StageRestoring before it moves any of its
//     objects, and the removal and the purge act on no pack at that stage. The
//     command runs one step at a time on a scratch store, holding a lock
//     beside it, so a purge never reads a pack removed that a restore is
//     bringing back.
//   - Pack ids are never reused, so a removed pack's keys name no later pack.
//   - The purge deletes, by version id, only the objects of a pack removed
//     for longer than its wait, and never falls back to a plain delete, which
//     on a bucket that keeps old versions hides the current version and
//     leaves it billed. A pack's record turns purged only after every
//     version of its keys is gone. No step that deletes an object falls back
//     to a plain delete: a move deletes its source by version id, and a store
//     that cannot list versions fails the step with the object kept.
//
// The scratch store's keys:
//
//	c/p/{pack id}            catalogued pack: blocks, value bytes, size, created
//	c/m/{pack id}/{tail}     a key the pack holds: value size, flags
//	c/r/{tail}               the key's refs: state, then each child ref
//	y/head                   the cycle's header
//	y/s/{pack id}            a pack registered at S: size, created
//	y/w/v/{tail}             a key in L
//	y/w/f/{tail}             a key on the walk's frontier: its ref
//	y/w/x/{tail}             a hole: reason, then its ref
//	y/j/{pack id}            the judgment of a pack
//
// A verify and a check each keep their own scratch store, apart from any
// cycle's:
//
//	v/v/{tail}               a key the verify read
//	v/f/{tail}               a key on the verify's frontier: its ref
//	x/k/{tail}               a key a candidate's index holds: the candidates, one id a line
//	x/v/{tail}               a key the check reached
//	x/f/{tail}               a key on the check's frontier: its ref
//
// A tail is the last 16 bytes of a block key, as the pack index's h rows key
// blocks.
package packsweep

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
// A tail is the last 16 bytes of a block key, as the pack index's h rows key
// blocks.
package packsweep

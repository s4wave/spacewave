// Package world_block stores a World as a block graph and serves it through
// one Engine.
//
// The World root block holds the object tree, the graph tree, the head of the
// change log and the per-type object counts. Both trees are block-backed
// key/value trees (kvtx/block), so a read fetches only the pages its keys
// reach and a commit rewrites only the paths it changed. The World's format
// and the implementation tags of its two trees must agree, and a root whose
// tags disagree is refused at load, before any read or write
// (NewestWorldFormat). The change log keeps between one and two segments of
// ChangeLogSegmentLen entries, so a watcher catches up on recent changes
// without reading the whole history.
//
// The Engine publishes each head as an immutable snapshot that readers pin
// without its lock; a reader keeps the storage of the head it pinned after a
// newer head replaces it. A write transaction holds the Authority's turn
// until its commit is admitted, then releases it so the next writer builds
// on the admitted candidate while this one waits for the decision. The
// engine installs a head only once the Authority has accepted it and it is
// durable; heads other writers commit arrive through the Authority's head
// watch. Close releases
// the engine's cursors and transactions; Retire also ends its transactions
// with ErrEngineRetired, a supersession, so callers rerun on the successor.
package world_block

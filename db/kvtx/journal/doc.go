// Package kvtx_journal makes a key-value store's commits durable in a remote
// journal instead of the store's own flush.
//
// A Store wraps a store with ordered commits, such as SQLite's, whose flush is
// the slow part of a commit. While the journal is live, each write
// transaction commits to the inner store without the flush and sends its
// writes, in order, as one record to a Sink. Commit returns once every commit
// up to it is durable: flushed locally, or its record flushed by the sink. The
// inner store still flushes in the background, and each flush tells the sink
// it may drop the records it covers.
//
// A host crash can lose the commits the inner store had not flushed. Open
// replays the sink's records past the store's durable head, in order,
// stopping at the first the sink lacks, before anything reads the store, then
// flushes them and has the sink set aside every other record of that run.
// Replay is exact because every write transaction while the journal is live
// is journaled: the commits a crash loses are a suffix of the inner store's
// commit order, and each is a record.
//
// The journal is relied on only while the Head marker in the store says it
// is live. The marker is set, and flushed, before the first commit that
// relies on the sink, and cleared only once every commit is flushed locally.
// So a store whose durable head has no live marker lost nothing the sink
// holds, and Open neither waits for the sink nor replays.
//
// Invariants, as NodePush.tla in block_store_nodepush models them:
//
//   - Commit returns nil only once every commit up to its own is durable
//     locally or in the sink (HeadDurable).
//   - Records carry consecutive seqs, numbered under the inner store's writer
//     turn, so a record's seq is its commit's place in the commit order; a
//     failed commit takes no seq.
//   - Every write transaction opened while the marker is live is journaled,
//     so the commits a crash loses are all records.
//   - Open applies only records of the run the durable head names, past its
//     seq, and stops at the first missing one.
//   - The marker is cleared only after a flush that covers every commit.
package kvtx_journal

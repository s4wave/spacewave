// Package writelog keeps an append-only log of the blocks a store accepts,
// the roots it sets and the blocks it fills from its remote, one line each:
//
//	<seq> <kind> <b58 hash>      kind: p put, r root, f fill
//
// A Log writes epochs. An epoch starts each time the log turns on, with a
// random id, and numbers its lines from one. It is split into segment files
// named {epoch}-{first seq}.wlog, each opening with an epoch line,
// "<first seq> e <epoch>", and the epoch's last segment ends with a close
// line, "<last seq> c <epoch>", when the log turns off or the process stops
// cleanly.
//
// Lines are written through a buffer and flushed in groups, with no fsync, so
// a crash loses the buffered tail. A reader treats an epoch with no close
// line, once its writer is gone, as a gap. A write that fails, or a directory
// past its byte cap, ends the epoch with no close line and turns the log off;
// the store's write is never failed for the log.
//
// A clean close lets the epoch resume, so a restart of the process is no gap.
// The close syncs the epoch's segments and leaves a resume marker naming its
// close line. A later Set in the same directory takes the marker and, when the
// epoch's last segment still ends with that close line and the log missed no
// write since, continues the epoch: same id, the next line number, in a new
// segment. A reader goes on past a close line into a segment that starts at
// the line after it. The log misses a write when one is recorded while no
// epoch is open, or when its store reports writes it did not show (Missed);
// the first miss removes the marker, so the next start opens a new epoch and
// every reader of the old one sees a gap.
//
// Invariant: an epoch's lines from any position name every block the store
// accepted, every root it set and every block it filled after that position,
// or the reader answers a gap. A marker exists only while the epoch is durable
// through its close line and no write escaped it since.
//
// What a write waits for: while the log is off, two atomic loads; the first
// miss after a close removes the marker and syncs the directory, once. A
// write already past the on check when the epoch closes waits for the close,
// including its syncs; one arriving while the log turns on waits for the new
// segment's creation.
package writelog

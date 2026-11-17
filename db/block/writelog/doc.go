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
// Turning the log off and on again in one process resumes from memory. Close,
// called once the store takes no more writes, also leaves a resume marker
// naming the close line, after the segments are durable: each finished
// segment is synced off the writer's path as it is retired, so Close waits on
// the last one's sync, the marker's and the directory's. The next process
// calls Claim before its store takes a write, with the log already the store's
// observer, which takes the marker away, durably, whether or not its log turns
// on; a Claim by a log no store observes records a miss. Its Set then
// continues the epoch
// when the last segment still ends with that close line and the log missed no
// write since: same id, the next line number, in a new segment. A reader goes
// on past a close line into a segment that starts at the line after it. The
// log misses a write when one is recorded while no epoch is open, or when its
// store reports writes it did not show (Missed); a miss keeps the epoch from
// resuming, retires a marker Close left, and ends an open epoch with no close
// line. A marker that cannot be removed durably poisons the epoch's last
// segment instead, so no start resumes it, and Close after a miss retries the
// removal. A miss and Close's marker write are ordered by a lock, so a miss
// either stops the write or retires the marker after it.
//
// Invariant: an epoch's lines from any position name every block the store
// accepted, every root it set and every block it filled after that position,
// or the reader answers a gap. A marker exists only while the epoch is durable
// through its close line, no process has claimed the directory since, and no
// write escaped the epoch. A process that writes the store without Claim, such
// as a build that predates it or an offline tool, breaks the invariant; remove
// the marker before running one. So does a process killed between a write
// accepted after Close and that write's retirement of the marker, which the
// Close contract forbids, and one whose directory refused both the marker's
// removal and the poison until it died.
//
// What a write waits for: while the log is on, nothing more than the buffered
// append; while it is off, two atomic loads, and the first miss does one
// failed open, or, after Close, removes the marker and syncs the directory. A
// write already past the on check when the epoch closes waits for the close
// line's flush, and at Close for the three syncs above.
package writelog

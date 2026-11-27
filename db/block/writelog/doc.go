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
// a process crash keeps them and a host crash loses the buffered tail. A
// reader treats an epoch with no close line, once its writer is gone, as a
// gap. A write that fails, or a directory past its byte cap, ends the epoch
// with no close line and turns the log off; the store's write is never failed
// or delayed for the log.
package writelog

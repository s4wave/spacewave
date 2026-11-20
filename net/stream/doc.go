// Package stream is the data channel two peers hold over a link.
//
// A Stream is reliable and ordered, with deadlines, and Close ends both
// directions and returns a pending Read or Write. OpenOpts.Unreliable opens a
// MessageStream instead: each Write is one message, delivered at most once and
// possibly out of order, and its Control stream stays open beside it for
// whatever the protocol needs delivered reliably.
//
// The subpackages build protocols on streams: srpc carries starpc RPC calls,
// echo, forwarding, listening, relay, datagram and packet move bytes, and
// netconn adapts a stream to net.Conn.
package stream

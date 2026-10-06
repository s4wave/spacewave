// Package s4db is a single-file embedded key-value engine.
//
// Every commit appends one record to a log inside the file. Each process
// keeps the records since the last checkpoint in an in-memory overlay above a
// copy-on-write B+tree, which a checkpoint rewrites once for many commits.
// Values above a size threshold live outside the tree, packed into extents
// whose pages are punched out of the file as soon as no snapshot reads them.
//
// Any number of processes may open the file. Each holds a reader slot that
// publishes the oldest state its snapshots read, and tails the log when the
// file changes. A write transaction holds the writer lock, so commits from
// all processes form one sequence.
package s4db

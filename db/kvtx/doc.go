// Package kvtx defines transactional key/value stores.
//
// A Store opens a Tx, read-only unless asked to write. A Tx is used from one
// goroutine at a time and always ends with Commit or Discard; a write Tx
// takes effect only through Commit, so one that ends in Discard changes
// nothing. Implementations range from in-memory maps to block-backed trees:
// package block stores a key/value tree as blocks, in an IAVL or Okra layout
// named by its KeyValueStore root, and package cayley serves a quad graph on
// any of them.
package kvtx

// Package bucket defines a namespace of blocks inside a Volume and the
// configuration that governs it.
//
// A Bucket serves the block operations under one bucket ID. Its Config, whose
// Rev rises with each change, carries the default put options, such as the
// hash type, and the lookup configuration that decides where a read searches
// after the local stores miss. An ObjectRef addresses a graph by its root
// BlockRef, the bucket that holds it, and the transform configuration that
// decodes it. Empty fields inherit the parent's, so a graph can cross buckets
// and transforms at any ref.
//
// Package lookup builds the Cursor that follows ObjectRefs across buckets and
// the Lookup that a read goes through. Package lookup/concurrent searches
// every bucket handle at once and, configured to, asks the network through
// the dex.LookupBlockFromNetwork directive. A block it recovers is written
// back as a cache fill only when its outgoing refs are known, so a bucket
// never holds a block whose edges were lost.
package bucket

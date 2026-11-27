package paylog

import "github.com/s4wave/spacewave/net/hash"

// Options configures a Store.
type Options struct {
	// HashType is the hash type of new block references. Zero selects the
	// default hash type.
	HashType hash.HashType
	// EqualWriters admits a write transaction opened with block.WithBackground
	// in arrival order with the foreground transactions, preserving the store
	// ordering from before writers were ordered by class.
	EqualWriters bool
}

package paylog

import "github.com/s4wave/spacewave/net/hash"

// Options configures a Store.
type Options struct {
	// HashType is the hash type of new block references; zero selects the
	// default.
	HashType hash.HashType
	// EqualWriters admits a write transaction opened with block.WithBackground
	// in arrival order with the foreground ones, as the store did before it
	// ordered its writers by class.
	EqualWriters bool
}

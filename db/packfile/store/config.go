package store

import "github.com/s4wave/spacewave/db/block"

// Config fixes a PackfileStore's lookup and resident-memory policy at
// construction.
// The store copies the settings. Changing this record does not reconfigure
// the store.
type Config struct {
	// RangeCacheMaxBytes bounds resident spans across every reader. Zero uses
	// the default resident budget. A negative value disables span eviction.
	RangeCacheMaxBytes int64
	// IndexByteLimit bounds resident indexes across every reader. Zero uses
	// the default index budget. A negative value keeps every index loaded.
	IndexByteLimit int64
	// ParsedIndex selects parsed indexes when true and in-place indexes when
	// false. Nil uses SPACEWAVE_PACKFILE_PARSED_INDEX's process default.
	ParsedIndex *bool
	// ReadCacheBudget replaces both local limits when non-nil, using one shared
	// budget for encoded spans, indexes, and the other caches that joined it.
	// The store leaves this budget when Close begins draining its readers.
	ReadCacheBudget *block.ReadCacheBudget
	// Locator places block keys ahead of bloom lookup. Nil uses only blooms.
	Locator Locator
}

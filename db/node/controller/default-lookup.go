package node_controller

import (
	lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
)

// BuildDefaultLookupConfig builds a new default lookup config.
func BuildDefaultLookupConfig() lookup.Config {
	return &lookup_concurrent.Config{}
}

// BuildConcurrentLookupConfig builds a new concurrent lookup config.
func BuildConcurrentLookupConfig(notFoundBehavior lookup_concurrent.NotFoundBehavior) lookup.Config {
	return &lookup_concurrent.Config{NotFoundBehavior: notFoundBehavior}
}

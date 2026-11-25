package packcache

// Counts are a Cache's running totals.
type Counts struct {
	// Hits counts Gets that found a value.
	Hits uint64
	// Misses counts Gets that found none, those before the load included.
	Misses uint64
	// Evicted counts packs evicted.
	Evicted uint64
	// Carried counts values carried forward by evictions.
	Carried uint64
}

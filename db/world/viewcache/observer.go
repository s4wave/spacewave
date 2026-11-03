package viewcache

import "time"

// Observer receives cache metrics. Implementations must be safe for concurrent
// calls and must not block on I/O or call back into the cache.
type Observer interface {
	// Computed records a completed computation under its key family and path.
	Computed(family, path string, elapsed time.Duration)
	// Dropped records why a retained value was invalidated.
	Dropped(family, cause string)
	// HeadRead records a revision lag and the reason a head read waited.
	HeadRead(lag uint64, wait string)
}

package packcache

import "github.com/s4wave/spacewave/db/packfile/packahead"

// Options configures a Cache.
type Options struct {
	// LimitBytes bounds the local packs' total size; the evictor works the
	// Log back under it. Zero keeps every pack.
	LimitBytes int64
	// Log configures the Log the cache keeps.
	Log packahead.Options
}

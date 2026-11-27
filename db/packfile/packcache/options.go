package packcache

import (
	"context"

	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// Options configures a Cache.
type Options struct {
	// LimitBytes bounds the local packs' total size. The evictor works the
	// Log back under it. Zero keeps every pack.
	LimitBytes int64
	// Log configures the Log the cache keeps.
	Log packahead.Options
	// Recent names keys read before this start, such as a list saved at the
	// last shutdown, by calling mark with bounded slices of them. The cache
	// calls it once, after the Log loads and before its first eviction, and
	// the evictor then carries those values forward as it carries values read
	// since their write. The Log forgets its reads at a restart, so without it
	// a cache that opens over a lowered limit evicts by write order alone.
	// Nil names none.
	Recent func(ctx context.Context, mark func(keys [][]byte))
}

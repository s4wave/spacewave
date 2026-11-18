//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"golang.org/x/sync/singleflight"
)

// coldTail is an authenticated, immutable kvfile index reader.
type coldTail struct {
	// reader searches the raw tail in place; it owns the only copy of those bytes.
	reader *kvfile.Reader
	// bytes counts raw tail bytes for cache accounting.
	bytes int
	// base is the boundary between payload extents and tail metadata.
	base uint64
}

// locate requires an exact, in-bounds key and extent.
func (t *coldTail) locate(key []byte) (off, length uint64, hit bool, err error) {
	// Use the same kvfile index search as packfile readers, over resident bytes.
	e, _, err := t.reader.SearchIndexEntryWithKey(key)
	if err != nil || e == nil {
		return 0, 0, false, err
	}
	if e.Size == 0 || e.Size > math.MaxInt || e.Offset >= t.base || e.Size > t.base-e.Offset {
		return 0, 0, false, errors.New("cold pack index points outside the payloads")
	}
	return e.Offset, e.Size, true, nil
}

// coldTailCache caches opened tails by byte cost and admits non-blocking
// look-ahead from existence probes. The worker and demand paths share the
// cache but not an in-flight fetch: a foreground read never waits on the worker.
type coldTailCache struct {
	// store supplies the cold bucket and counters.
	store *ColdPackStore
	// limit caps retained raw tail bytes.
	limit int
	// mtx guards the pending fill set; it never guards I/O or parsing.
	mtx sync.Mutex
	// resident holds verified tails in LRU order.
	resident *coldCache[uint16, *coldTail]
	// pending holds admitted pack numbers until the worker finishes each.
	pending map[uint16]struct{}
	// fillCh is the bounded queue for background-only requests.
	fillCh chan uint16
	// demand deduplicates foreground tail loads only.
	demand singleflight.Group
}

// newColdTailCache creates the cache and its bounded background queue.
func newColdTailCache(store *ColdPackStore, limit int) *coldTailCache {
	return &coldTailCache{
		store: store, limit: limit, resident: newColdCache[uint16, *coldTail](limit),
		pending: make(map[uint16]struct{}), fillCh: make(chan uint16, 64),
	}
}

// get touches a resident tail without doing any I/O.
func (c *coldTailCache) get(pack uint16) *coldTail {
	// Retain a reference independent of later eviction.
	tail, _ := c.resident.get(pack)
	return tail
}

// queue offers one fill without waiting for the worker, network or disk.
func (c *coldTailCache) queue(pack uint16) {
	// Coalesce offers for one pack without retaining unadmitted keys.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if _, queued := c.pending[pack]; queued || c.get(pack) != nil {
		return
	}
	select {
	case c.fillCh <- pack:
		c.pending[pack] = struct{}{}
	default:
		c.store.stats.tailFillDropped.Add(1)
	}
}

// fill handles existence-driven tail warming, one range at a time.
func (c *coldTailCache) fill(ctx context.Context, catalog *coldPackCatalog, warm []uint16) {
	// Pace recovered-pack warming; explicit existence requests take priority.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	var tick <-chan time.Time
	if len(warm) != 0 {
		tick = timer.C
	}
	for ctx.Err() == nil {
		// Run one background fetch at a time on its own range lane.
		select {
		case <-ctx.Done():
			return
		case pack := <-c.fillCh:
			c.backgroundFetch(ctx, catalog, pack, false)
			c.mtx.Lock()
			delete(c.pending, pack)
			c.mtx.Unlock()
		case <-tick:
			start := time.Now()
			if !coldPackReadsOff.Load() && len(c.fillCh) == 0 && len(c.store.ranges) == 0 {
				if !c.resident.room(catalog.packs[warm[0]].meta.IndexTailBytes) {
					tick = nil // Startup warming never evicts a resident tail.
					continue
				}
				c.backgroundFetch(ctx, catalog, warm[0], true)
				warm = warm[1:]
			}
			if len(warm) == 0 {
				tick = nil
				continue
			}
			timer.Reset(max(100*time.Millisecond, 2*time.Since(start)))
		}
	}
}

// backgroundFetch warms one pack without blocking or sharing a flight with demand.
func (c *coldTailCache) backgroundFetch(ctx context.Context, catalog *coldPackCatalog, num uint16, spareOnly bool) {
	// Never fetch a gone, already cached, disabled, or oversized pack tail.
	pack := &catalog.packs[num]
	if coldPackReadsOff.Load() || pack.gone.Load() || c.get(num) != nil || !c.cacheable(pack.meta.IndexTailBytes) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, coldReadLimit)
	defer cancel()
	if _, err := c.fetch(ctx, pack, num, true, spareOnly); err != nil {
		c.store.warn(ctx, err, "fill cold pack index tail")
	}
}

// cacheable reports whether a raw tail can ever fit the LRU.
func (c *coldTailCache) cacheable(size uint64) bool {
	return size <= uint64(c.limit) //nolint:gosec // The constructor requires a positive limit.
}

// read loads a tail on a foreground miss, never joining background work.
func (c *coldTailCache) read(ctx context.Context, catalog *coldPackCatalog, pack uint16) (*coldTail, error) {
	// Coalesce foreground requests for this immutable pack's tail.
	ch := c.demand.DoChan(strconv.Itoa(int(pack)), func() (any, error) {
		// Recheck the cache after a preceding demand or background load.
		if tail := c.get(pack); tail != nil {
			return tail, nil
		}
		return c.fetch(ctx, &catalog.packs[pack], pack, false, false)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.(*coldTail), nil
	}
}

// fetch uses packfile's suffix reader, with a cold-specific first-read cap.
func (c *coldTailCache) fetch(ctx context.Context, pack *coldPack, num uint16, background, spareOnly bool) (*coldTail, error) {
	// Size the first read without overflow; forbid a corrupt trailer from
	// requesting bytes outside the verified tail or the fixed first read.
	meta := pack.meta
	size, count := meta.Entry.SizeBytes, meta.Entry.BlockCount
	const maxFirst = 2 << 20
	first := uint64(maxFirst)
	if count <= (maxFirst-4096)/64 {
		first = count*64 + 4096
	}
	base := size - min(size, max(first, meta.IndexTailBytes))
	raw, err := packfile.ReadIndexTail(ctx, size, first, func(ctx context.Context, off int64, length int) ([]byte, error) {
		// Never use a trailer-supplied offset to read the entire pack.
		if off < 0 || uint64(off) < base {
			return nil, errors.New("cold index tail exceeds manifest bounds")
		}
		return c.store.readRange(ctx, pack, off, length, background, true)
	})
	if err != nil {
		return nil, err
	}

	// Only the tail checked by the offline builder can authorize a positive.
	digest := sha256.Sum256(raw)
	if uint64(len(raw)) != meta.IndexTailBytes || !bytes.Equal(digest[:], meta.IndexSha256) {
		return nil, errors.New("cold index tail does not match manifest")
	}
	reader, err := kvfile.BuildReaderWithIndexTail(bytes.Clone(raw), size)
	if err != nil {
		return nil, err
	}
	if reader.Size() != count {
		return nil, errors.New("cold index tail count does not match manifest")
	}
	tail := &coldTail{reader: reader, bytes: len(raw), base: size - uint64(len(raw))}

	// Keep a tail only when it fits the LRU; a foreground caller may still use it.
	c.resident.put(num, tail, tail.bytes, spareOnly)
	return tail, nil
}

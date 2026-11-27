//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/singleflight"
)

// coldMoveTailLimit bounds the mover's extra cache without shrinking the
// configured user cache or fetching the same stored tail for every block.
const coldMoveTailLimit = 16 << 20

// coldKeySize is the encoded SHA-256 hash without extension fields.
const coldKeySize = 4 + sha256.Size

// coldKeyPrefix is the generated writer encoding before the SHA-256 bytes.
var coldKeyPrefix = [coldKeySize - sha256.Size]byte(packfile.BlockKey(&hash.Hash{
	HashType: hash.RecommendedHashType, Hash: make([]byte, sha256.Size),
}))

// coldTailEntryBytes is the size of a retained digest and two uint64s.
const coldTailEntryBytes = sha256.Size + 8 + 8

// coldTailEntry pairs an entire SHA-256 digest with its verified index extent.
type coldTailEntry struct {
	// digest excludes only the identical prefix of each canonical SHA-256 key.
	digest [sha256.Size]byte
	// off is the byte offset of the encoded block value in the pack.
	off uint64
	// size is the length of the encoded value.
	size uint64
}

// coldTail is an authenticated, immutable index. It keeps the decoded index
// or the raw reader, never both. Only complete keys can authorize presence.
type coldTail struct {
	// entries contains exact canonical digests in sorted order.
	entries []coldTailEntry
	// reader is used when a pack is too large or has an extended key encoding.
	reader *kvfile.Reader
	// base is the payload boundary for the raw reader.
	base uint64
}

// coldTailCharge bounds both the compact index and the raw fallback.
func coldTailCharge(meta *packfile.ColdPackEntry) uint64 {
	// A hostile declared count must never wrap and admit a huge allocation.
	count := meta.Entry.BlockCount
	if count > math.MaxUint64/coldTailEntryBytes {
		return math.MaxUint64
	}
	return max(meta.IndexTailBytes, count*coldTailEntryBytes)
}

// decodeColdTail builds a compact index using kvfile's decoder. It returns nil
// without error for an extended key so those packs keep their exact wire keys.
func decodeColdTail(ctx context.Context, r *kvfile.Reader, base uint64) ([]coldTailEntry, error) {
	// Decode only a charge-bounded tail. No parsed entry retains raw bytes.
	entries := make([]coldTailEntry, r.Size())
	for i := range entries {
		// A corrupt extent or canceled fill must not publish a partial index.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		e, err := r.ReadIndexEntry(uint64(i)) //nolint:gosec // i is a nonnegative slice index.
		if err != nil {
			return nil, err
		}
		if !coldTailExtentOK(e, base) {
			return nil, errors.New("cold pack index points outside the payloads")
		}

		// Canonical keys share one prefix, so their digest order is wire-key order.
		if len(e.Key) != coldKeySize || !bytes.Equal(e.Key[:len(coldKeyPrefix)], coldKeyPrefix[:]) {
			return nil, nil
		}
		digest := [sha256.Size]byte(e.Key[len(coldKeyPrefix):])
		if i != 0 && bytes.Compare(entries[i-1].digest[:], digest[:]) >= 0 {
			return nil, errors.New("cold pack index keys are not strictly ordered")
		}
		entries[i] = coldTailEntry{digest: digest, off: e.Offset, size: e.Size}
	}
	return entries, nil
}

// coldTailExtentOK requires a nonempty extent inside the pack body.
func coldTailExtentOK(e *kvfile.IndexEntry, base uint64) bool {
	return e.Size != 0 && e.Size <= math.MaxInt && e.Offset < base && e.Size <= base-e.Offset
}

// locate searches the resident immutable index without allocating. Oversized
// and extended-key tails fall back to the original exact kvfile search.
func (t *coldTail) locate(key []byte) (off, length uint64, hit bool, err error) {
	// Never equate an extended wire key with a canonical full digest.
	if t.entries != nil {
		if len(key) != coldKeySize || !bytes.Equal(key[:len(coldKeyPrefix)], coldKeyPrefix[:]) {
			return 0, 0, false, nil
		}
		// Pass the digest by value so the comparator cannot retain caller scratch.
		digest := [sha256.Size]byte(key[len(coldKeyPrefix):])
		i, ok := slices.BinarySearchFunc(t.entries, digest, func(e coldTailEntry, key [sha256.Size]byte) int {
			return bytes.Compare(e.digest[:], key[:])
		})
		if !ok {
			return 0, 0, false, nil
		}
		e := t.entries[i]
		return e.off, e.size, true, nil
	}

	// The raw reader still verifies the selected extent before returning it.
	e, _, err := t.reader.SearchIndexEntryWithKey(key)
	if err != nil || e == nil {
		return 0, 0, false, err
	}
	if !coldTailExtentOK(e, t.base) {
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
	// mtx guards the pending fill set. It never guards I/O or parsing.
	mtx sync.Mutex
	// resident holds user and warming tails. A move never evicts or touches them.
	resident *coldCache[uint16, *coldTail]
	// moveResident retains move fills under an independent byte budget.
	moveResident *coldCache[uint16, *coldTail]
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
		store: store, resident: newColdCache[uint16, *coldTail](limit),
		moveResident: newColdCache[uint16, *coldTail](min(limit, coldMoveTailLimit)),
		pending:      make(map[uint16]struct{}), fillCh: make(chan uint16, 64),
	}
}

// get touches a resident tail without doing any I/O.
func (c *coldTailCache) get(pack uint16) *coldTail {
	// Retain a reference independent of later eviction.
	tail, _ := c.resident.get(pack)
	return tail
}

// getForMove reuses its own cached tail or borrows a user tail without
// changing the user's eviction order.
func (c *coldTailCache) getForMove(pack uint16) *coldTail {
	// A move can use either immutable copy, but only touches its own LRU.
	if tail, ok := c.moveResident.get(pack); ok {
		return tail
	}
	tail, _ := c.resident.peek(pack)
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
	// Pace recovered-pack warming. Explicit existence requests take
	// priority.
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
				if !c.resident.room(coldTailCharge(catalog.packs[warm[0]].meta)) {
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
	if coldPackReadsOff.Load() || pack.unusable() || c.get(num) != nil || !c.cacheable(coldTailCharge(pack.meta)) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, coldReadLimit)
	defer cancel()
	if _, err := c.fetch(ctx, pack, num, coldBackgroundRange, spareOnly); err != nil {
		c.store.warn(ctx, err, "fill cold pack index tail")
	}
}

// cacheable reports whether an index can ever fit the LRU.
func (c *coldTailCache) cacheable(size uint64) bool {
	return size <= uint64(c.resident.limit) //nolint:gosec // The constructor requires a positive limit.
}

// read loads a tail on a foreground miss, never joining background work.
func (c *coldTailCache) read(ctx context.Context, catalog *coldPackCatalog, pack uint16, moving bool) (*coldTail, error) {
	// Moves may queue for I/O. Never let a user flight join that wait.
	if moving {
		if tail := c.getForMove(pack); tail != nil {
			return tail, nil
		}
		return c.fetch(ctx, &catalog.packs[pack], pack, coldMoveRange, false)
	}

	// Coalesce foreground requests for this immutable pack's tail.
	ch := c.demand.DoChan(strconv.Itoa(int(pack)), func() (any, error) {
		// Recheck the cache after a preceding demand or background load.
		if tail := c.get(pack); tail != nil {
			return tail, nil
		}

		// The first waiter does not own the lifetime of the shared request.
		flight, cancel := context.WithTimeout(context.WithoutCancel(ctx), coldReadLimit)
		defer cancel()
		return c.fetch(flight, &catalog.packs[pack], pack, coldUserRange, false)
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
func (c *coldTailCache) fetch(ctx context.Context, pack *coldPack, num uint16, lane coldRangeLane, spareOnly bool) (*coldTail, error) {
	// Size the first read without overflow. Forbid a corrupt trailer from
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
		return c.store.readRange(ctx, pack, off, length, lane, true)
	})
	if err != nil {
		return nil, err
	}

	// Only the tail checked by the offline builder can locate an extent.
	digest := sha256.Sum256(raw)
	if uint64(len(raw)) != meta.IndexTailBytes || !bytes.Equal(digest[:], meta.IndexSha256) {
		return nil, errors.New("cold index tail does not match manifest")
	}

	// Check the reader agrees with the manifest on the number of extents.
	reader, err := kvfile.BuildReaderWithIndexTail(raw, size)
	if err != nil {
		return nil, err
	}
	if reader.Size() != count {
		return nil, errors.New("cold index tail count does not match manifest")
	}

	// A move's fill must never displace a user tail. Use the selected
	// cache's budget before decoding. An oversized index uses the exact
	// raw reader.
	cache := c.resident
	if lane == coldMoveRange {
		cache = c.moveResident
	}
	base = size - uint64(len(raw))
	tail := &coldTail{reader: reader, base: base}
	charge := coldTailCharge(meta)
	if charge > uint64(cache.limit) { //nolint:gosec // Both cache limits are positive.
		return tail, nil
	}

	// Decode the authenticated bytes once, outside the cache lock. Retained
	// indexes carry every exact key and its checked extent, not the raw tail.
	entries, err := decodeColdTail(ctx, reader, base)
	if err != nil {
		return nil, err
	}
	if entries != nil {
		tail = &coldTail{entries: entries}
		charge = count * coldTailEntryBytes
	} else {
		// Copy only this tail because the ranged response backing it
		// can be larger.
		tail.reader, err = kvfile.BuildReaderWithIndexTail(bytes.Clone(raw), size)
		if err != nil {
			return nil, err
		}
		charge = uint64(len(raw))
	}

	// A canceled load must not publish even a completely decoded index.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache.put(num, tail, int(charge), spareOnly) //nolint:gosec // The charge did not grow after the cacheability check.
	return tail, nil
}

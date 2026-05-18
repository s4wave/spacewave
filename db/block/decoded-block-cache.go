package block

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/dgraph-io/ristretto/v2"
	ristrettoz "github.com/dgraph-io/ristretto/v2/z"
	"github.com/pkg/errors"
)

const (
	// DecodedBlockCacheNoTransformKey identifies untransformed decoded-cache entries.
	DecodedBlockCacheNoTransformKey = "transform:none"
	decodedBlockCacheTrustKey       = "trust:verified-block-ref"
)

// DecodedBlockCacheable identifies a block type for decoded-block caching.
type DecodedBlockCacheable interface {
	DecodedBlockCacheTypeKey() string
}

// DecodedBlockCacheTransformer identifies a transform boundary for decoded-block caching.
type DecodedBlockCacheTransformer interface {
	DecodedBlockCacheTransformKey() string
}

type decodedBlockCacheSizer interface {
	SizeVT() int
}

const decodedBlockCacheEntryOverheadCost int64 = 256

// DecodedBlockCache owns shared decoded block reuse.
type DecodedBlockCache struct {
	cache *ristretto.Cache[string, Block]
	opts  DecodedBlockCacheOptions

	mtx   sync.Mutex
	byRef map[string]map[string]struct{}
	// byHash lets async Ristretto callbacks prune byRef; rejected or evicted
	// entries must not leave old refs pinned in the invalidation index.
	byHash map[decodedBlockCacheHash]decodedBlockCacheTrackedKey
}

type decodedBlockCacheKey struct {
	ref       string
	blockType string
	transform string
	trust     string
}

type decodedBlockCacheHash struct {
	key      uint64
	conflict uint64
}

type decodedBlockCacheTrackedKey struct {
	ref string
	key string
}

// NewDecodedBlockCache constructs a decoded-block cache with default options.
func NewDecodedBlockCache() *DecodedBlockCache {
	cache, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		panic(err)
	}
	return cache
}

// NewDecodedBlockCacheWithOptions constructs a decoded-block cache owner.
func NewDecodedBlockCacheWithOptions(opts DecodedBlockCacheOptions) (*DecodedBlockCache, error) {
	opts = opts.normalize()
	cache := &DecodedBlockCache{
		opts:   opts,
		byRef:  make(map[string]map[string]struct{}),
		byHash: make(map[decodedBlockCacheHash]decodedBlockCacheTrackedKey),
	}
	if opts.Disabled {
		return cache, nil
	}
	db, err := ristretto.NewCache(&ristretto.Config[string, Block]{
		NumCounters: opts.NumCounters,
		MaxCost:     opts.MaxCost,
		BufferItems: opts.BufferItems,
		Metrics:     true,
		OnEvict: func(item *ristretto.Item[Block]) {
			cache.removeRefKeyHash(decodedBlockCacheHash{key: item.Key, conflict: item.Conflict})
		},
		OnReject: func(item *ristretto.Item[Block]) {
			cache.removeRefKeyHash(decodedBlockCacheHash{key: item.Key, conflict: item.Conflict})
		},
	})
	if err != nil {
		return nil, err
	}
	cache.cache = db
	return cache, nil
}

// WithDecodedBlockCache attaches an owner-provided decoded-block cache to ctx.
func WithDecodedBlockCache(ctx context.Context, cache *DecodedBlockCache) context.Context {
	if cache == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, decodedBlockCacheContextKey{}, cache)
}

// MaxCost returns the configured decoded-cache budget.
func (c *DecodedBlockCache) MaxCost() int64 {
	if c == nil {
		return 0
	}
	if c.cache != nil {
		return c.cache.MaxCost()
	}
	return c.opts.MaxCost
}

// Wait blocks until buffered cache writes have reached Ristretto.
func (c *DecodedBlockCache) Wait() {
	if c == nil || c.cache == nil {
		return
	}
	c.cache.Wait()
}

// Close releases the cache goroutine.
func (c *DecodedBlockCache) Close() {
	if c == nil {
		return
	}
	c.mtx.Lock()
	c.byRef = nil
	c.byHash = nil
	c.mtx.Unlock()
	if c.cache == nil {
		return
	}
	c.cache.Close()
}

// Snapshot returns shared decoded-cache metrics.
func (c *DecodedBlockCache) Snapshot() DecodedBlockCacheSnapshot {
	if c == nil {
		return DecodedBlockCacheSnapshot{}
	}
	snapshot := DecodedBlockCacheSnapshot{MaxCost: c.MaxCost()}
	if c.cache == nil {
		return snapshot
	}
	snapshot.RemainingCost = c.cache.RemainingCost()
	snapshot.RetainedCost = snapshot.MaxCost - snapshot.RemainingCost
	metrics := c.cache.Metrics
	if metrics == nil {
		return snapshot
	}
	snapshot.Hits = metrics.Hits()
	snapshot.Misses = metrics.Misses()
	snapshot.Stores = metrics.KeysAdded() + metrics.KeysUpdated()
	snapshot.Rejections = metrics.SetsDropped() + metrics.SetsRejected()
	snapshot.Evictions = metrics.KeysEvicted()
	snapshot.CostAdded = metrics.CostAdded()
	snapshot.CostEvicted = metrics.CostEvicted()
	return snapshot
}

func (c *DecodedBlockCache) Lookup(ctx context.Context, front *decodedBlockFrontCache, key decodedBlockCacheKey) (Block, bool, error) {
	if cached := front.lookup(key); cached != nil {
		cloned, ok, err := cloneDecodedBlock(cached)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			RecordDecodedBlockUncloneable(ctx)
			return nil, false, nil
		}
		RecordDecodedBlockCacheHit(ctx, true)
		return cloned, true, nil
	}
	if c == nil || c.cache == nil {
		if front != nil {
			recordDecodedBlockCacheMiss(ctx)
		}
		return nil, false, nil
	}
	cached, ok := c.cache.Get(key.String())
	if !ok {
		recordDecodedBlockCacheMiss(ctx)
		return nil, false, nil
	}
	front.store(key, cached)
	cloned, cloneOK, err := cloneDecodedBlock(cached)
	if err != nil {
		return nil, false, err
	}
	if !cloneOK {
		RecordDecodedBlockUncloneable(ctx)
		return nil, false, nil
	}
	RecordDecodedBlockCacheHit(ctx, true)
	return cloned, true, nil
}

func (c *DecodedBlockCache) Store(
	ctx context.Context,
	front *decodedBlockFrontCache,
	key decodedBlockCacheKey,
	ref *BlockRef,
	blk Block,
	data []byte,
) error {
	if blk == nil {
		return nil
	}
	if c == nil && front == nil {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}
	if ref == nil || ref.VerifyData(data, false) != nil {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}
	cloned, ok, err := cloneDecodedBlock(blk)
	if err != nil {
		return err
	}
	if !ok {
		RecordDecodedBlockUncloneable(ctx)
		return nil
	}
	front.store(key, cloned)
	if c == nil || c.cache == nil {
		return nil
	}
	cost, ok := decodedBlockCacheCost(blk, data)
	if !ok {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}
	cacheKey := key.String()
	// Ristretto can reject asynchronously, so record before Set and let reject
	// callbacks remove entries that never become durable cache contents.
	c.recordRefKey(key.ref, cacheKey)
	accepted := c.cache.Set(cacheKey, cloned, cost)
	recordDecodedBlockCacheStore(ctx, accepted, cost)
	if !accepted {
		c.removeRefKey(cacheKey)
		recordDecodedBlockCacheRejected(ctx)
		return nil
	}
	return nil
}

// InvalidateRef removes decoded cache entries for ref.
func (c *DecodedBlockCache) InvalidateRef(ctx context.Context, ref *BlockRef) {
	refKey, ok := decodedBlockCacheRefKey(ref)
	if !ok {
		return
	}
	decodedBlockFrontCacheFromContext(ctx).invalidateRef(refKey)
	if c == nil || c.cache == nil {
		return
	}
	keys := c.takeRefKeys(refKey)
	for key := range keys {
		c.cache.Del(key)
	}
}

func (c *DecodedBlockCache) recordRefKey(refKey, key string) {
	if c == nil || refKey == "" || key == "" {
		return
	}
	c.mtx.Lock()
	if c.byRef == nil {
		c.byRef = make(map[string]map[string]struct{})
	}
	if c.byHash == nil {
		c.byHash = make(map[decodedBlockCacheHash]decodedBlockCacheTrackedKey)
	}
	keys := c.byRef[refKey]
	if keys == nil {
		keys = make(map[string]struct{})
		c.byRef[refKey] = keys
	}
	keys[key] = struct{}{}
	h := decodedBlockCacheHashFor(key)
	c.byHash[h] = decodedBlockCacheTrackedKey{ref: refKey, key: key}
	c.mtx.Unlock()
}

func (c *DecodedBlockCache) removeRefKey(key string) {
	c.removeRefKeyHash(decodedBlockCacheHashFor(key))
}

func (c *DecodedBlockCache) removeRefKeyHash(h decodedBlockCacheHash) {
	if c == nil {
		return
	}
	c.mtx.Lock()
	tracked, ok := c.byHash[h]
	if ok {
		delete(c.byHash, h)
		keys := c.byRef[tracked.ref]
		delete(keys, tracked.key)
		if len(keys) == 0 {
			delete(c.byRef, tracked.ref)
		}
	}
	c.mtx.Unlock()
}

func (c *DecodedBlockCache) takeRefKeys(refKey string) map[string]struct{} {
	if c == nil {
		return nil
	}
	c.mtx.Lock()
	keys := c.byRef[refKey]
	delete(c.byRef, refKey)
	for key := range keys {
		delete(c.byHash, decodedBlockCacheHashFor(key))
	}
	c.mtx.Unlock()
	return keys
}

func decodedBlockCacheHashFor(key string) decodedBlockCacheHash {
	keyHash, conflictHash := ristrettoz.KeyToHash[string](key)
	return decodedBlockCacheHash{key: keyHash, conflict: conflictHash}
}

func invalidateDecodedBlockRef(ctx context.Context, cache *DecodedBlockCache, ref *BlockRef) {
	cache.InvalidateRef(ctx, ref)
}

func decodedBlockCacheRefKey(ref *BlockRef) (string, bool) {
	if ref == nil || ref.GetEmpty() {
		return "", false
	}
	refKey, err := ref.MarshalKey()
	if err != nil || len(refKey) == 0 {
		return "", false
	}
	return string(refKey), true
}

func lookupDecodedBlock(ctx context.Context, key decodedBlockCacheKey) (Block, bool, error) {
	return decodedBlockCacheFromContext(ctx).Lookup(ctx, decodedBlockFrontCacheFromContext(ctx), key)
}

func storeDecodedBlock(ctx context.Context, key decodedBlockCacheKey, ref *BlockRef, blk Block, data []byte) error {
	return decodedBlockCacheFromContext(ctx).Store(ctx, decodedBlockFrontCacheFromContext(ctx), key, ref, blk, data)
}

type decodedBlockCacheContextKey struct{}

func decodedBlockCacheFromContext(ctx context.Context) *DecodedBlockCache {
	if ctx != nil {
		if cache, _ := ctx.Value(decodedBlockCacheContextKey{}).(*DecodedBlockCache); cache != nil {
			return cache
		}
	}
	return nil
}

func decodedBlockFrontCacheFromContext(ctx context.Context) *decodedBlockFrontCache {
	op := readOperationContextFromContext(ctx)
	if op == nil {
		return nil
	}
	return op.decodedBlocks
}

func decodedBlockCacheCost(blk Block, data []byte) (int64, bool) {
	rawCost := int64(len(data))
	if rawCost <= 0 {
		return 0, false
	}
	decodedCost := int64(0)
	if sizer, ok := blk.(decodedBlockCacheSizer); ok {
		decodedCost = int64(sizer.SizeVT())
	}
	if decodedCost <= 0 {
		decodedData, err := blk.MarshalBlock()
		if err != nil {
			return 0, false
		}
		decodedCost = int64(len(decodedData))
	}
	if decodedCost <= 0 {
		return 0, false
	}
	return rawCost + decodedCost + decodedBlockCacheEntryOverheadCost, true
}

func decodedBlockCacheKeyFor(ref *BlockRef, blk Block, xfrm Transformer) (decodedBlockCacheKey, bool) {
	if ref == nil || ref.GetEmpty() || blk == nil {
		return decodedBlockCacheKey{}, false
	}
	typeKeyer, ok := blk.(DecodedBlockCacheable)
	if !ok {
		return decodedBlockCacheKey{}, false
	}
	blockType := typeKeyer.DecodedBlockCacheTypeKey()
	if blockType == "" {
		return decodedBlockCacheKey{}, false
	}
	transform, ok := decodedBlockCacheTransformKey(xfrm)
	if !ok {
		return decodedBlockCacheKey{}, false
	}
	refKey, ok := decodedBlockCacheRefKey(ref)
	if !ok {
		return decodedBlockCacheKey{}, false
	}
	return decodedBlockCacheKey{
		ref:       refKey,
		blockType: blockType,
		transform: transform,
		trust:     decodedBlockCacheTrustKey,
	}, true
}

func decodedBlockCacheTransformKey(xfrm Transformer) (string, bool) {
	if xfrm == nil {
		return DecodedBlockCacheNoTransformKey, true
	}
	keyer, ok := xfrm.(DecodedBlockCacheTransformer)
	if !ok {
		return "", false
	}
	key := keyer.DecodedBlockCacheTransformKey()
	if key == "" {
		return "", false
	}
	return key, true
}

func (k decodedBlockCacheKey) String() string {
	var b strings.Builder
	writePart := func(part string) {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	writePart(k.ref)
	writePart(k.blockType)
	writePart(k.transform)
	writePart(k.trust)
	return b.String()
}

func cloneDecodedBlock(blk Block) (Block, bool, error) {
	cloned, err := CloneBlock(blk)
	if err != nil {
		if errors.Is(err, ErrNotClonable) || errors.Is(err, ErrUnexpectedType) {
			return nil, false, nil
		}
		return nil, false, err
	}
	out, ok := cloned.(Block)
	if !ok {
		return nil, false, nil
	}
	return out, true, nil
}

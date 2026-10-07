package block

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/pkg/errors"
)

const (
	// DecodedBlockCacheNoTransformKey identifies untransformed decoded-cache entries.
	DecodedBlockCacheNoTransformKey = "transform:none"
	// decodedBlockCacheTrustKey marks entries decoded from a verified block ref.
	decodedBlockCacheTrustKey = "trust:verified-block-ref"
)

// DecodedBlockCacheable identifies a block type for decoded-block caching.
type DecodedBlockCacheable interface {
	// DecodedBlockCacheTypeKey returns the block type's cache identity.
	DecodedBlockCacheTypeKey() string
}

// DecodedBlockCacheTransformer identifies a transform boundary for decoded-block caching.
type DecodedBlockCacheTransformer interface {
	// DecodedBlockCacheTransformKey returns the transform's cache identity.
	DecodedBlockCacheTransformKey() string
}

// DecodedBlockHeapSizer reports the heap bytes a decoded block retains. The
// decoded-block cache charges this size against its budget. Implement it for
// block types whose decoded form is much larger than their encoding, such as
// pages of many small messages.
type DecodedBlockHeapSizer interface {
	// DecodedHeapSize returns the heap bytes the decoded block retains.
	DecodedHeapSize() int
}

// decodedBlockCacheSizer reports a decoded block's size without marshaling it.
type decodedBlockCacheSizer interface {
	// SizeVT returns the block's encoded size.
	SizeVT() int
}

// decodedBlockCacheEntryOverheadCost charges Ristretto's per-entry map and
// admission bookkeeping.
const decodedBlockCacheEntryOverheadCost int64 = 256

// decodedBlockCacheStripes is the number of ref invalidation epochs per
// scope. InvalidateRef advances the stripe its ref hashes to, so other refs in
// that stripe miss once; the fixed array bounds invalidation state.
const decodedBlockCacheStripes = 4096

// decodedBlockPool is a Ristretto cache and cost budget shared by scopes.
type decodedBlockPool struct {
	// cache holds decoded entries; nil when caching is disabled.
	cache *ristretto.Cache[string, decodedBlockCacheEntry]
	// maxCost is the configured budget.
	maxCost int64
	// nextScope allocates scope key prefixes.
	nextScope atomic.Uint64
}

// sharedDecodedBlockPool is the process-wide pool behind NewDecodedBlockCache.
// It lives for the process, so one budget bounds every block store.
var sharedDecodedBlockPool = sync.OnceValue(func() *decodedBlockPool {
	pool, err := newDecodedBlockPool(DefaultDecodedBlockCacheOptions())
	if err != nil {
		panic(err)
	}
	return pool
})

// newDecodedBlockPool constructs a pool with opts.
func newDecodedBlockPool(opts DecodedBlockCacheOptions) (*decodedBlockPool, error) {
	// Normalize the decoded-cache budget and preserve disabled pools.
	opts = opts.normalize()
	pool := &decodedBlockPool{maxCost: opts.MaxCost}
	if opts.Disabled {
		return pool, nil
	}

	// Construct the shared cache with the configured cost limits.
	cache, err := ristretto.NewCache(&ristretto.Config[string, decodedBlockCacheEntry]{
		NumCounters: opts.NumCounters,
		MaxCost:     opts.MaxCost,
		BufferItems: opts.BufferItems,
		Metrics:     true,
	})
	if err != nil {
		return nil, err
	}

	// Attach the configured cache to its decoded-block pool.
	pool.cache = cache
	return pool, nil
}

// DecodedBlockCache is one owner's scope in a decoded-block pool. Keys carry
// the scope prefix, so owners never see each other's blocks. Each entry
// records the invalidation epochs current when it was stored; Lookup treats an
// entry with an outdated epoch as a miss, and stale entries age out of the
// pool through Ristretto.
type DecodedBlockCache struct {
	// pool holds the entries and budget.
	pool *decodedBlockPool
	// ownsPool is set when Close must close pool.
	ownsPool bool
	// scope prefixes every key of this owner.
	scope string
	// closed disables lookups and stores after Close.
	closed atomic.Bool
	// clearEpoch advances on InvalidateAll and Close.
	clearEpoch atomic.Uint64
	// refEpochs advance on InvalidateRef, one per stripe.
	refEpochs [decodedBlockCacheStripes]atomic.Uint64
}

// decodedBlockCacheKey identifies one decoded form of a block: its ref, type,
// transform, and trust.
type decodedBlockCacheKey struct {
	ref       string
	blockType string
	transform string
	trust     string
}

// decodedBlockCacheEpochs identifies the invalidation state an entry was
// stored under.
type decodedBlockCacheEpochs struct {
	ref   uint64
	clear uint64
}

// decodedBlockCacheEntry is a cached block and the epochs it was stored under.
type decodedBlockCacheEntry struct {
	block  Block
	epochs decodedBlockCacheEpochs
}

// decodedBlockCacheStoreToken captures the epochs at the start of a read so a
// store after an intervening invalidation is skipped.
type decodedBlockCacheStoreToken struct {
	cache  *DecodedBlockCache
	epochs decodedBlockCacheEpochs
	ok     bool
}

// NewDecodedBlockCache constructs a scope in the process-wide pool.
func NewDecodedBlockCache() *DecodedBlockCache {
	return newDecodedBlockCacheScope(sharedDecodedBlockPool(), false)
}

// NewDecodedBlockCacheWithOptions constructs a scope in a private pool with
// opts. Close releases the pool.
func NewDecodedBlockCacheWithOptions(opts DecodedBlockCacheOptions) (*DecodedBlockCache, error) {
	pool, err := newDecodedBlockPool(opts)
	if err != nil {
		return nil, err
	}
	return newDecodedBlockCacheScope(pool, true), nil
}

// newDecodedBlockCacheScope allocates a scope in pool.
func newDecodedBlockCacheScope(pool *decodedBlockPool, ownsPool bool) *DecodedBlockCache {
	id := pool.nextScope.Add(1)
	return &DecodedBlockCache{
		pool:     pool,
		ownsPool: ownsPool,
		scope:    strconv.FormatUint(id, 36) + "/",
	}
}

// WithDecodedBlockCache attaches a decoded-block cache to ctx.
func WithDecodedBlockCache(ctx context.Context, cache *DecodedBlockCache) context.Context {
	if cache == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, decodedBlockCacheContextKey{}, cache)
}

// MaxCost returns the pool's decoded-cache budget.
func (c *DecodedBlockCache) MaxCost() int64 {
	if c == nil {
		return 0
	}
	return c.pool.maxCost
}

// Wait blocks until buffered pool writes have reached Ristretto.
func (c *DecodedBlockCache) Wait() {
	if c == nil || c.pool.cache == nil {
		return
	}
	c.pool.cache.Wait()
}

// Close ends the scope. Its entries in a shared pool become stale and age out.
func (c *DecodedBlockCache) Close() {
	if c == nil || c.closed.Swap(true) {
		return
	}
	c.clearEpoch.Add(1)
	if c.ownsPool && c.pool.cache != nil {
		c.pool.cache.Close()
	}
}

// Snapshot returns the pool's decoded-cache metrics. A scope in the shared
// pool reports the metrics of every scope.
func (c *DecodedBlockCache) Snapshot() DecodedBlockCacheSnapshot {
	// Return an empty metrics snapshot for a missing cache scope.
	if c == nil {
		return DecodedBlockCacheSnapshot{}
	}

	// Capture the pool budget before reading enabled cache metrics.
	snapshot := DecodedBlockCacheSnapshot{MaxCost: c.MaxCost()}
	if c.pool.cache == nil {
		return snapshot
	}

	// Capture the cache cost and locate its metrics counters.
	snapshot.RemainingCost = c.pool.cache.RemainingCost()
	snapshot.RetainedCost = snapshot.MaxCost - snapshot.RemainingCost
	metrics := c.pool.cache.Metrics
	if metrics == nil {
		return snapshot
	}

	// Capture the pool hit, store, rejection, and eviction counters.
	snapshot.Hits = metrics.Hits()
	snapshot.Misses = metrics.Misses()
	snapshot.Stores = metrics.KeysAdded() + metrics.KeysUpdated()
	snapshot.Rejections = metrics.SetsDropped() + metrics.SetsRejected()
	snapshot.Evictions = metrics.KeysEvicted()
	snapshot.CostAdded = metrics.CostAdded()
	snapshot.CostEvicted = metrics.CostEvicted()
	return snapshot
}

// Lookup returns the cached decoded block for the key, recording a cache
// hit or miss. The returned block is a clone safe for the caller to keep.
func (c *DecodedBlockCache) Lookup(ctx context.Context, front *decodedBlockFrontCache, key decodedBlockCacheKey) (Block, bool, error) {
	// Reuse the decoded block retained in the read-operation cache.
	if cached := front.lookup(key); cached != nil {
		return cloneDecodedBlockHit(ctx, cached)
	}

	// Record a miss when the shared decoded-block pool is disabled.
	if !c.enabled() {
		if front != nil {
			recordDecodedBlockCacheMiss(ctx)
		}
		return nil, false, nil
	}

	// Look up the shared entry and reject stale invalidation epochs.
	cacheKey := c.cacheKey(key)
	cached, ok := c.pool.cache.Get(cacheKey)
	if ok && cached.epochs != c.epochs(key.ref) {
		c.pool.cache.Del(cacheKey)
		ok = false
	}

	// Record a cache miss when no current shared entry exists.
	if !ok {
		recordDecodedBlockCacheMiss(ctx)
		return nil, false, nil
	}

	// Retain the shared block in the read cache and clone it for the caller.
	front.store(key, cached.block)
	return cloneDecodedBlockHit(ctx, cached.block)
}

// Store caches the decoded block under the key when the store token is
// still current.
func (c *DecodedBlockCache) Store(
	ctx context.Context,
	front *decodedBlockFrontCache,
	token decodedBlockCacheStoreToken,
	key decodedBlockCacheKey,
	ref *BlockRef,
	blk Block,
	data []byte,
) error {
	// Require a block and a cache scope before retaining decoded content.
	if blk == nil {
		return nil
	}
	if c == nil && front == nil {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}

	// Require verified stored bytes before caching their decoded block.
	if ref == nil || ref.VerifyData(data, false) != nil {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}

	// Clone the decoded block so cache readers cannot mutate it.
	cloned, ok, err := cloneDecodedBlock(blk)
	if err != nil {
		return err
	}
	if !ok {
		RecordDecodedBlockUncloneable(ctx)
		return nil
	}

	// Reject cache writes whose read token predates an invalidation.
	var epochs decodedBlockCacheEpochs
	if c != nil {
		epochs = c.epochs(key.ref)
		if token.ok && (token.cache != c || token.epochs != epochs) {
			return nil
		}
	}

	// Retain the cloned block in the read-operation cache.
	front.store(key, cloned)
	if !c.enabled() {
		return nil
	}

	// Compute the shared entry cost before cache admission.
	cacheKey := c.cacheKey(key)
	cost, ok := decodedBlockCacheCost(cacheKey, blk, data)
	if !ok {
		RecordDecodedBlockUncacheable(ctx)
		return nil
	}

	// An invalidation after the epoch read leaves this entry stale, and
	// Lookup then treats it as a miss.
	accepted := c.pool.cache.Set(cacheKey, decodedBlockCacheEntry{
		block:  cloned,
		epochs: epochs,
	}, cost)
	recordDecodedBlockCacheStore(ctx, accepted, cost)
	if !accepted {
		recordDecodedBlockCacheRejected(ctx)
	}
	return nil
}

// InvalidateRef makes cached entries for ref stale.
func (c *DecodedBlockCache) InvalidateRef(ctx context.Context, ref *BlockRef) {
	// Resolve the block reference to invalidate in both cache layers.
	refKey, ok := decodedBlockCacheRefKey(ref)
	if !ok {
		return
	}

	// Invalidate the read cache and advance the shared reference epoch.
	decodedBlockFrontCacheFromContext(ctx).invalidateRef(refKey)
	if c == nil {
		return
	}
	c.refEpoch(refKey).Add(1)
}

// InvalidateAll makes every cached entry of this scope stale.
func (c *DecodedBlockCache) InvalidateAll(ctx context.Context) {
	decodedBlockFrontCacheFromContext(ctx).clear()
	if c == nil {
		return
	}
	c.clearEpoch.Add(1)
}

// enabled reports whether the scope can read and write its pool.
func (c *DecodedBlockCache) enabled() bool {
	return c != nil && c.pool.cache != nil && !c.closed.Load()
}

// epochs returns the current invalidation epochs for refKey.
func (c *DecodedBlockCache) epochs(refKey string) decodedBlockCacheEpochs {
	return decodedBlockCacheEpochs{
		ref:   c.refEpoch(refKey).Load(),
		clear: c.clearEpoch.Load(),
	}
}

// cacheKey returns the pool key for key in this scope: the scope, then the
// reference, block type, transform, and trust identities, each length-prefixed
// so no part can be confused with a neighbor. Every lookup builds one, so it
// allocates once.
func (c *DecodedBlockCache) cacheKey(key decodedBlockCacheKey) string {
	// Reserve the scope plus each part with its longest decimal length prefix.
	parts := [...]string{key.ref, key.blockType, key.transform, key.trust}
	size := len(c.scope)
	for _, part := range parts {
		size += len(part) + decodedBlockCacheKeyPrefixLen
	}

	// Encode the scope and the length-prefixed parts.
	var b strings.Builder
	b.Grow(size)
	b.WriteString(c.scope)
	var prefix [decodedBlockCacheKeyPrefixLen]byte
	for _, part := range parts {
		b.Write(strconv.AppendInt(prefix[:0], int64(len(part)), 10))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

// decodedBlockCacheKeyPrefixLen bounds one part's length prefix: the decimal
// digits of an int64 and the separator.
const decodedBlockCacheKeyPrefixLen = 20

// storeToken captures the current epochs for refKey.
func (c *DecodedBlockCache) storeToken(refKey string) decodedBlockCacheStoreToken {
	if c == nil || refKey == "" {
		return decodedBlockCacheStoreToken{}
	}
	return decodedBlockCacheStoreToken{cache: c, epochs: c.epochs(refKey), ok: true}
}

// refEpoch returns the invalidation epoch of the stripe refKey hashes to.
// Ref keys encode content hashes, so unseeded FNV-1a spreads them evenly; a
// collision only invalidates more entries than needed.
func (c *DecodedBlockCache) refEpoch(refKey string) *atomic.Uint64 {
	h := uint64(14695981039346656037)
	for i := range len(refKey) {
		h ^= uint64(refKey[i])
		h *= 1099511628211
	}
	return &c.refEpochs[h%decodedBlockCacheStripes]
}

// cloneDecodedBlockHit clones a cached block for the caller and records the
// hit, or reports a miss when the block cannot be cloned.
func cloneDecodedBlockHit(ctx context.Context, cached Block) (Block, bool, error) {
	// Clone the cached block and classify unavailable clone support.
	cloned, ok, err := cloneDecodedBlock(cached)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		RecordDecodedBlockUncloneable(ctx)
		return nil, false, nil
	}

	// Record the successful decoded-cache hit for the caller.
	RecordDecodedBlockCacheHit(ctx, true)
	return cloned, true, nil
}

// decodedBlockCacheRefKey marshals the ref into its cache key prefix.
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

// lookupDecodedBlock looks the key up in the context's caches.
func lookupDecodedBlock(ctx context.Context, key decodedBlockCacheKey) (Block, bool, error) {
	return decodedBlockCacheFromContext(ctx).Lookup(ctx, decodedBlockFrontCacheFromContext(ctx), key)
}

// decodedBlockCacheStoreTokenFromContext returns a store token bound to
// the context's cache and ref.
func decodedBlockCacheStoreTokenFromContext(ctx context.Context, refKey string) decodedBlockCacheStoreToken {
	return decodedBlockCacheFromContext(ctx).storeToken(refKey)
}

// storeDecodedBlock stores the block in the context's caches.
func storeDecodedBlock(
	ctx context.Context,
	key decodedBlockCacheKey,
	token decodedBlockCacheStoreToken,
	ref *BlockRef,
	blk Block,
	data []byte,
) error {
	return decodedBlockCacheFromContext(ctx).Store(ctx, decodedBlockFrontCacheFromContext(ctx), token, key, ref, blk, data)
}

// decodedBlockCacheContextKey is the context key of the attached cache.
type decodedBlockCacheContextKey struct{}

// decodedBlockCacheFromContext returns the shared decoded block cache
// from the context, or nil.
func decodedBlockCacheFromContext(ctx context.Context) *DecodedBlockCache {
	if ctx != nil {
		if cache, _ := ctx.Value(decodedBlockCacheContextKey{}).(*DecodedBlockCache); cache != nil {
			return cache
		}
	}
	return nil
}

// decodedBlockFrontCacheFromContext returns the read operation's front
// cache from the context, or nil.
func decodedBlockFrontCacheFromContext(ctx context.Context) *decodedBlockFrontCache {
	op := readOperationContextFromContext(ctx)
	if op == nil {
		return nil
	}
	return op.decodedBlocks
}

// decodedBlockCacheCost computes the cost of a cached entry: its pool key, the
// heap its decoded block retains, and a fixed charge for Ristretto's
// bookkeeping. A block without DecodedBlockHeapSizer is charged its raw plus
// encoded decoded size as a heap estimate. It returns false when the block
// size is unknown.
func decodedBlockCacheCost(cacheKey string, blk Block, data []byte) (int64, bool) {
	// Charge the reported heap size when the block knows it.
	keyCost := int64(len(cacheKey)) + decodedBlockCacheEntryOverheadCost
	if sizer, ok := blk.(DecodedBlockHeapSizer); ok {
		if heapCost := int64(sizer.DecodedHeapSize()); heapCost > 0 {
			return keyCost + heapCost, true
		}
	}

	// Otherwise estimate the heap from the raw and decoded sizes.
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
	return keyCost + rawCost + decodedCost, true
}

// decodedBlockCacheKeyFor builds the cache key for a block, or false
// when the block type is not cacheable.
func decodedBlockCacheKeyFor(ref *BlockRef, blk Block, xfrm Transformer) (decodedBlockCacheKey, bool) {
	// Require a nonempty reference and block for cache identity.
	if ref == nil || ref.GetEmpty() || blk == nil {
		return decodedBlockCacheKey{}, false
	}

	// Resolve the concrete block type used by the decoded cache.
	typeKeyer, ok := blk.(DecodedBlockCacheable)
	if !ok {
		return decodedBlockCacheKey{}, false
	}
	blockType := typeKeyer.DecodedBlockCacheTypeKey()
	if blockType == "" {
		return decodedBlockCacheKey{}, false
	}

	// Resolve the transform identity used by the decoded cache.
	transform, ok := decodedBlockCacheTransformKey(xfrm)
	if !ok {
		return decodedBlockCacheKey{}, false
	}

	// Encode the block reference used by the decoded cache.
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

// decodedBlockCacheTransformKey returns the transformer's cache identity,
// or the no-transform key.
func decodedBlockCacheTransformKey(xfrm Transformer) (string, bool) {
	// Use the untransformed cache identity when no transformer applies.
	if xfrm == nil {
		return DecodedBlockCacheNoTransformKey, true
	}

	// Require a transformer with a nonempty decoded-cache identity.
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

// cloneDecodedBlock deep-clones a block, returning false when the block
// type cannot be cloned.
func cloneDecodedBlock(blk Block) (Block, bool, error) {
	// Clone the decoded block and distinguish unsupported block types.
	cloned, err := CloneBlock(blk)
	if err != nil {
		if errors.Is(err, ErrNotClonable) || errors.Is(err, ErrUnexpectedType) {
			return nil, false, nil
		}
		return nil, false, err
	}

	// Require the clone to preserve the block interface.
	out, ok := cloned.(Block)
	if !ok {
		return nil, false, nil
	}
	return out, true, nil
}

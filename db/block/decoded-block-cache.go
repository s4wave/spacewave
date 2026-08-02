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
	cache *ristretto.Cache[string, decodedBlockCacheEntry]
	opts  DecodedBlockCacheOptions

	mtx   sync.Mutex
	byRef map[string]map[string]struct{}
	// byHash lets async Ristretto callbacks prune byRef; rejected or evicted
	// entries must not leave old refs pinned in the invalidation index.
	byHash     map[decodedBlockCacheHash]map[uint64]decodedBlockCacheTrackedKey
	refEpoch   map[string]uint64
	clearEpoch uint64
	generation uint64
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
	ref        string
	key        string
	generation uint64
}

type decodedBlockCacheEntry struct {
	block      Block
	generation uint64
}

type decodedBlockCacheStoreToken struct {
	cache      *DecodedBlockCache
	ref        string
	refEpoch   uint64
	clearEpoch uint64
	ok         bool
}

// NewDecodedBlockCache constructs a decoded-block cache with default options.
func NewDecodedBlockCache() *DecodedBlockCache {
	cache, err := NewDecodedBlockCacheWithOptions(DefaultDecodedBlockCacheOptions())
	if err != nil {
		panic(err)
	}
	return cache
}

// NewDecodedBlockCacheWithOptions constructs a decoded-block cache with opts.
func NewDecodedBlockCacheWithOptions(opts DecodedBlockCacheOptions) (*DecodedBlockCache, error) {
	opts = opts.normalize()
	cache := &DecodedBlockCache{
		opts:     opts,
		byRef:    make(map[string]map[string]struct{}),
		byHash:   make(map[decodedBlockCacheHash]map[uint64]decodedBlockCacheTrackedKey),
		refEpoch: make(map[string]uint64),
	}
	if opts.Disabled {
		return cache, nil
	}
	db, err := ristretto.NewCache(&ristretto.Config[string, decodedBlockCacheEntry]{
		NumCounters: opts.NumCounters,
		MaxCost:     opts.MaxCost,
		BufferItems: opts.BufferItems,
		Metrics:     true,
		OnEvict: func(item *ristretto.Item[decodedBlockCacheEntry]) {
			cache.removeRefKeyHashGeneration(
				decodedBlockCacheHash{key: item.Key, conflict: item.Conflict},
				item.Value.generation,
			)
		},
		OnReject: func(item *ristretto.Item[decodedBlockCacheEntry]) {
			cache.removeRefKeyHashGeneration(
				decodedBlockCacheHash{key: item.Key, conflict: item.Conflict},
				item.Value.generation,
			)
		},
	})
	if err != nil {
		return nil, err
	}
	cache.cache = db
	return cache, nil
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
	c.refEpoch = nil
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
	cacheKey := key.String()
	cached, ok := c.cache.Get(cacheKey)
	if !ok {
		recordDecodedBlockCacheMiss(ctx)
		return nil, false, nil
	}
	c.compactRefKeyGenerations(key.ref, cacheKey, cached.generation)
	front.store(key, cached.block)
	cloned, cloneOK, err := cloneDecodedBlock(cached.block)
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
	token decodedBlockCacheStoreToken,
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
	if c != nil && !c.storeTokenCurrent(token) {
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
	_, replacing := c.cache.Get(cacheKey)
	// Ristretto can reject asynchronously, so record before Set and let reject
	// callbacks remove entries that never become durable cache contents.
	c.mtx.Lock()
	if !c.storeTokenCurrentLocked(token) {
		c.mtx.Unlock()
		front.invalidateRef(key.ref)
		return nil
	}
	generation := c.recordRefKeyLocked(key.ref, cacheKey)
	accepted := c.cache.Set(cacheKey, decodedBlockCacheEntry{
		block:      cloned,
		generation: generation,
	}, cost)
	if !accepted {
		c.removeRefKeyHashGenerationLocked(decodedBlockCacheHashFor(cacheKey), generation)
	}
	if accepted && replacing {
		// Ristretto updates replace the stored value immediately without an old
		// entry eviction callback. Keep the side index on the resident generation.
		c.compactRefKeyGenerationsLocked(decodedBlockCacheHashFor(cacheKey), key.ref, cacheKey, generation)
	}
	c.mtx.Unlock()
	recordDecodedBlockCacheStore(ctx, accepted, cost)
	if !accepted {
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
	if c == nil {
		return
	}
	keys := c.takeRefKeys(refKey)
	if c.cache == nil {
		return
	}
	for key := range keys {
		c.cache.Del(key)
	}
}

// InvalidateAll removes every decoded cache entry owned by c.
func (c *DecodedBlockCache) InvalidateAll(ctx context.Context) {
	decodedBlockFrontCacheFromContext(ctx).clear()
	if c == nil {
		return
	}
	keys := c.takeAllKeys()
	if c.cache == nil {
		return
	}
	for key := range keys {
		c.cache.Del(key)
	}
}

func (c *DecodedBlockCache) storeToken(refKey string) decodedBlockCacheStoreToken {
	if c == nil || refKey == "" {
		return decodedBlockCacheStoreToken{}
	}
	c.mtx.Lock()
	if c.refEpoch == nil {
		c.refEpoch = make(map[string]uint64)
	}
	token := decodedBlockCacheStoreToken{
		cache:      c,
		ref:        refKey,
		refEpoch:   c.refEpoch[refKey],
		clearEpoch: c.clearEpoch,
		ok:         true,
	}
	c.mtx.Unlock()
	return token
}

func (c *DecodedBlockCache) storeTokenCurrent(token decodedBlockCacheStoreToken) bool {
	c.mtx.Lock()
	ok := c.storeTokenCurrentLocked(token)
	c.mtx.Unlock()
	return ok
}

func (c *DecodedBlockCache) storeTokenCurrentLocked(token decodedBlockCacheStoreToken) bool {
	if !token.ok {
		return true
	}
	if token.cache != c {
		return false
	}
	return c.clearEpoch == token.clearEpoch && c.refEpoch[token.ref] == token.refEpoch
}

func (c *DecodedBlockCache) recordRefKeyLocked(refKey, key string) uint64 {
	if c == nil || refKey == "" || key == "" {
		return 0
	}
	if c.byRef == nil {
		c.byRef = make(map[string]map[string]struct{})
	}
	if c.byHash == nil {
		c.byHash = make(map[decodedBlockCacheHash]map[uint64]decodedBlockCacheTrackedKey)
	}
	keys := c.byRef[refKey]
	if keys == nil {
		keys = make(map[string]struct{})
		c.byRef[refKey] = keys
	}
	keys[key] = struct{}{}
	h := decodedBlockCacheHashFor(key)
	c.generation++
	generations := c.byHash[h]
	if generations == nil {
		generations = make(map[uint64]decodedBlockCacheTrackedKey)
		c.byHash[h] = generations
	}
	generations[c.generation] = decodedBlockCacheTrackedKey{
		ref:        refKey,
		key:        key,
		generation: c.generation,
	}
	return c.generation
}

func (c *DecodedBlockCache) removeRefKeyHashGeneration(h decodedBlockCacheHash, generation uint64) {
	if c == nil {
		return
	}
	c.mtx.Lock()
	c.removeRefKeyHashGenerationLocked(h, generation)
	c.mtx.Unlock()
}

func (c *DecodedBlockCache) removeRefKeyHashGenerationLocked(h decodedBlockCacheHash, generation uint64) {
	generations := c.byHash[h]
	tracked, ok := generations[generation]
	if !ok {
		return
	}
	delete(generations, generation)
	if len(generations) == 0 {
		delete(c.byHash, h)
	}
	// Ristretto can reject a duplicate admission while an older generation for
	// the same decoded key still owns the invalidation index. Only remove byRef
	// after the last tracked generation for this ref/key has left Ristretto.
	if c.hasRefKeyGenerationLocked(h, tracked.ref, tracked.key) {
		return
	}
	keys := c.byRef[tracked.ref]
	delete(keys, tracked.key)
	if len(keys) == 0 {
		delete(c.byRef, tracked.ref)
	}
}

func (c *DecodedBlockCache) compactRefKeyGenerations(ref, key string, generation uint64) {
	if c == nil {
		return
	}
	c.mtx.Lock()
	c.compactRefKeyGenerationsLocked(decodedBlockCacheHashFor(key), ref, key, generation)
	c.mtx.Unlock()
}

func (c *DecodedBlockCache) compactRefKeyGenerationsLocked(
	h decodedBlockCacheHash,
	ref string,
	key string,
	generation uint64,
) {
	for gen, tracked := range c.byHash[h] {
		if gen == generation || tracked.ref != ref || tracked.key != key {
			continue
		}
		delete(c.byHash[h], gen)
	}
	if len(c.byHash[h]) == 0 {
		delete(c.byHash, h)
	}
}

func (c *DecodedBlockCache) hasRefKeyGenerationLocked(h decodedBlockCacheHash, ref, key string) bool {
	for _, tracked := range c.byHash[h] {
		if tracked.ref == ref && tracked.key == key {
			return true
		}
	}
	return false
}

func (c *DecodedBlockCache) takeRefKeys(refKey string) map[string]struct{} {
	if c == nil {
		return nil
	}
	c.mtx.Lock()
	if c.refEpoch == nil {
		c.refEpoch = make(map[string]uint64)
	}
	c.refEpoch[refKey]++
	keys := c.byRef[refKey]
	delete(c.byRef, refKey)
	for key := range keys {
		delete(c.byHash, decodedBlockCacheHashFor(key))
	}
	c.mtx.Unlock()
	return keys
}

func (c *DecodedBlockCache) takeAllKeys() map[string]struct{} {
	if c == nil {
		return nil
	}
	c.mtx.Lock()
	c.clearEpoch++
	keys := make(map[string]struct{})
	for refKey, refKeys := range c.byRef {
		if c.refEpoch == nil {
			c.refEpoch = make(map[string]uint64)
		}
		c.refEpoch[refKey]++
		for key := range refKeys {
			keys[key] = struct{}{}
		}
	}
	c.byRef = make(map[string]map[string]struct{})
	c.byHash = make(map[decodedBlockCacheHash]map[uint64]decodedBlockCacheTrackedKey)
	c.mtx.Unlock()
	return keys
}

func decodedBlockCacheHashFor(key string) decodedBlockCacheHash {
	keyHash, conflictHash := ristrettoz.KeyToHash[string](key)
	return decodedBlockCacheHash{key: keyHash, conflict: conflictHash}
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

func decodedBlockCacheStoreTokenFromContext(ctx context.Context, refKey string) decodedBlockCacheStoreToken {
	return decodedBlockCacheFromContext(ctx).storeToken(refKey)
}

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

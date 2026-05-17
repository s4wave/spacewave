package block

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

const (
	decodedBlockCacheNoTransformKey = "transform:none"
	decodedBlockCacheTrustKey       = "trust:store-returned"
)

// DecodedBlockCacheable identifies a block type for decoded-block caching.
type DecodedBlockCacheable interface {
	DecodedBlockCacheTypeKey() string
}

// DecodedBlockCacheTransformer identifies a transform boundary for decoded-block caching.
type DecodedBlockCacheTransformer interface {
	DecodedBlockCacheTransformKey() string
}

// DecodedBlockCache owns decoded block reuse for one bounded owner lifetime.
type DecodedBlockCache struct {
	mtx     sync.Mutex
	entries map[decodedBlockCacheKey]Block
}

type decodedBlockCacheKey struct {
	ref       string
	blockType string
	transform string
	trust     string
}

// NewDecodedBlockCache constructs an empty decoded-block cache.
func NewDecodedBlockCache() *DecodedBlockCache {
	return &DecodedBlockCache{
		entries: make(map[decodedBlockCacheKey]Block),
	}
}

func newDecodedBlockCache() *DecodedBlockCache {
	return NewDecodedBlockCache()
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

func (c *DecodedBlockCache) Lookup(ctx context.Context, key decodedBlockCacheKey) (Block, bool, error) {
	if c == nil {
		recordDecodedBlockCacheMiss(ctx)
		return nil, false, nil
	}
	c.mtx.Lock()
	cached := c.entries[key]
	c.mtx.Unlock()
	if cached == nil {
		recordDecodedBlockCacheMiss(ctx)
		return nil, false, nil
	}
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

func (c *DecodedBlockCache) Store(ctx context.Context, key decodedBlockCacheKey, blk Block) error {
	if c == nil || blk == nil {
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
	c.mtx.Lock()
	c.entries[key] = cloned
	c.mtx.Unlock()
	return nil
}

type decodedBlockCacheContextKey struct{}

func decodedBlockCacheFromContext(ctx context.Context) *DecodedBlockCache {
	if ctx == nil {
		return nil
	}
	if cache, _ := ctx.Value(decodedBlockCacheContextKey{}).(*DecodedBlockCache); cache != nil {
		return cache
	}
	op := readOperationContextFromContext(ctx)
	if op == nil {
		return nil
	}
	return op.decodedBlocks
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
	refKey, err := ref.MarshalKey()
	if err != nil || len(refKey) == 0 {
		return decodedBlockCacheKey{}, false
	}
	return decodedBlockCacheKey{
		ref:       string(refKey),
		blockType: blockType,
		transform: transform,
		trust:     decodedBlockCacheTrustKey,
	}, true
}

func decodedBlockCacheTransformKey(xfrm Transformer) (string, bool) {
	if xfrm == nil {
		return decodedBlockCacheNoTransformKey, true
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

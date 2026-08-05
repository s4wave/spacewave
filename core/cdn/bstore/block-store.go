package cdn_bstore

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// DefaultPointerTTL is the fallback TTL for cached root pointers when the
// caller does not override it.
const DefaultPointerTTL = 30 * time.Second

// Options configure a CdnBlockStore.
type Options struct {
	// CdnBaseURL is the public CDN origin (e.g. https://cdn.spacewave.app).
	CdnBaseURL string
	// SpaceID is the CDN Space ULID.
	SpaceID string
	// HttpClient overrides the default http.Client.
	HttpClient *http.Client
	// PointerTTL is the cache TTL for the decoded root pointer. Zero falls
	// back to DefaultPointerTTL. Negative disables the TTL (pointer is cached
	// until explicitly invalidated).
	PointerTTL time.Duration
	// IndexCache optionally persists raw packfile index tails across restarts.
	IndexCache packfile_store.IndexCache
}

// PackIndexObjectStoreID returns the stable metadata ObjectStore ID for a CDN
// Space's durable packfile index cache.
func PackIndexObjectStoreID(spaceID string) string {
	return "cdn/" + spaceID + "/pack-index"
}

// CdnBlockStore is a read-only block.StoreOps backed by the public Spacewave
// CDN. Reads are served by a packfile_store.PackfileStore fed by an anonymous
// HTTP Range opener. Writes return ErrReadOnly. The cached root pointer is
// refreshed lazily when the TTL expires or Invalidate is called.
type CdnBlockStore struct {
	opts     Options
	cli      *http.Client
	opener   packfile_store.Opener
	memCache *memIndexCache
	pfs      *packfile_store.PackfileStore

	decodedBlocks   *block.DecodedBlockCache
	bcast           broadcast.Broadcast
	pointer         *cdn.CdnRootPointer
	pointerTime     time.Time
	pointerEpoch    uint64
	writebackTarget block.StoreOps
}

// NewCdnBlockStore constructs a new CdnBlockStore. The pointer is fetched
// lazily on the first read; pass a pre-populated pointer via SetPointer if
// the caller already has one.
func NewCdnBlockStore(opts Options) (*CdnBlockStore, error) {
	// Validate CDN configuration and select dependencies.
	if opts.CdnBaseURL == "" {
		return nil, errors.New("cdn bstore: CdnBaseURL required")
	}
	if opts.SpaceID == "" {
		return nil, errors.New("cdn bstore: SpaceID required")
	}
	cli := opts.HttpClient
	if cli == nil {
		cli = http.DefaultClient
	}

	// Configure index caching and the anonymous packfile opener.
	cache := opts.IndexCache
	var memCache *memIndexCache
	if cache == nil {
		memCache = newMemIndexCache()
		cache = memCache
	}
	opener := NewAnonymousOpener(cli, opts.CdnBaseURL, opts.SpaceID)
	pfs := packfile_store.NewPackfileStore(opener, cache)

	// Allocate the decoded-block cache.
	decodedBlocks, err := block.NewDecodedBlockCacheWithOptions(block.DefaultDecodedBlockCacheOptions())
	if err != nil {
		return nil, err
	}

	// Assemble the CDN block store.
	return &CdnBlockStore{
		opts:          opts,
		cli:           cli,
		opener:        opener,
		memCache:      memCache,
		pfs:           pfs,
		decodedBlocks: decodedBlocks,
	}, nil
}

// GetID returns the block store id; CDN block stores use the Space ULID
// verbatim because the mount is 1:1 with a Space.
func (s *CdnBlockStore) GetID() string {
	return s.opts.SpaceID
}

// GetHashType returns the preferred block hash type.
func (s *CdnBlockStore) GetHashType() hash.HashType {
	return s.pfs.GetHashType()
}

// GetSupportedFeatures returns the native feature bitset.
func (s *CdnBlockStore) GetSupportedFeatures() block.StoreFeature {
	return 0
}

// GetDecodedBlockCache returns the lifecycle-owned decoded-block cache.
func (s *CdnBlockStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return s.decodedBlocks
}

// BeginReadOperation returns the CDN block store as the scoped read handle.
func (s *CdnBlockStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// Close releases the decoded-block cache owned by this block store.
func (s *CdnBlockStore) Close() {
	if s.decodedBlocks == nil {
		return
	}
	s.decodedBlocks.Close()
	s.decodedBlocks = nil
}

// SetWriteback enables local co-block persistence through the underlying
// packfile store.
func (s *CdnBlockStore) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	// Publish writeback configuration and update packfile verification.
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		s.writebackTarget = target
		broadcastFn()
	})
	s.pfs.SetWriteback(ctx, target, windowBytes)
	s.pfs.SetVerifyBeforeServe(target != nil)
}

// SetRangeCacheMaxBytes sets the resident range-cache budget per pack reader.
func (s *CdnBlockStore) SetRangeCacheMaxBytes(maxBytes int64) {
	s.pfs.SetRangeCacheMaxBytes(maxBytes)
}

// GetBlock reads a block by reference, refreshing the pointer if needed.
func (s *CdnBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Serve a current decoded-cache hit when available.
	if data, ok, err := s.getCurrentCachedBlock(ctx, ref); err != nil || ok {
		return data, ok, err
	}

	// Read the block through the current CDN manifest.
	var data []byte
	var found bool
	err := s.withCurrentManifest(ctx, func() error {
		var err error
		data, found, err = s.pfs.GetBlock(ctx, ref)
		return err
	})
	return data, found, err
}

// GetBlockExists checks if a block exists.
func (s *CdnBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	var found bool
	err := s.withCurrentManifest(ctx, func() error {
		var err error
		found, err = s.pfs.GetBlockExists(ctx, ref)
		return err
	})
	return found, err
}

// GetBlockExistsBatch checks whether each block exists.
func (s *CdnBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	var found []bool
	err := s.withCurrentManifest(ctx, func() error {
		var err error
		found, err = s.pfs.GetBlockExistsBatch(ctx, refs)
		return err
	})
	return found, err
}

// StatBlock returns block metadata without reading the data.
func (s *CdnBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	var stat *block.BlockStat
	err := s.withCurrentManifest(ctx, func() error {
		var err error
		stat, err = s.pfs.StatBlock(ctx, ref)
		return err
	})
	return stat, err
}

// PutBlock is not supported on an anonymous CDN block store.
func (s *CdnBlockStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch is not supported on an anonymous CDN block store.
func (s *CdnBlockStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return block_store.ErrReadOnly
}

// RmBlock is not supported on an anonymous CDN block store.
func (s *CdnBlockStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync reports always-durable: the CDN block store holds no buffered writes.
func (s *CdnBlockStore) Sync(_ context.Context) (bool, error) {
	return true, nil
}

// Pointer returns the currently-cached root pointer without triggering a
// refresh. Returns nil if no pointer has been fetched yet.
func (s *CdnBlockStore) Pointer() *cdn.CdnRootPointer {
	var ptr *cdn.CdnRootPointer
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ptr = s.pointer
	})
	return ptr
}

// Refresh forces a re-fetch of the root pointer and updates the manifest.
// Returns the new pointer (nil if the CDN Space is empty).
func (s *CdnBlockStore) Refresh(ctx context.Context) (*cdn.CdnRootPointer, error) {
	// Fetch and publish the current CDN root pointer.
	ptr, err := FetchRootPointer(ctx, s.cli, s.opts.CdnBaseURL, s.opts.SpaceID)
	if err != nil {
		return nil, err
	}
	s.setPointer(ctx, ptr)
	return ptr, nil
}

// Invalidate drops the cached pointer so the next read re-fetches.
func (s *CdnBlockStore) Invalidate() {
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if s.memCache != nil {
			s.memCache.reset()
		}
		s.invalidateDecodedBlocks(context.Background())
		s.pfs.UpdateManifest(nil)
		s.pointer = nil
		s.pointerTime = time.Time{}
		s.pointerEpoch++
		broadcastFn()
	})
}

// SetPointer replaces the cached pointer without issuing a network request.
// Used by callers that receive a pointer via an external channel (for example
// the cdn-root-changed session WS frame which will land in Phase F).
func (s *CdnBlockStore) SetPointer(ptr *cdn.CdnRootPointer) {
	s.setPointer(context.Background(), ptr)
}

// EnsureDecodedBlockCacheFresh refreshes pointer state before decoded cache reads.
func (s *CdnBlockStore) EnsureDecodedBlockCacheFresh(ctx context.Context) error {
	// Decoded cache hits can otherwise bypass ensurePointer entirely. Keep CDN
	// pointer TTL freshness in CdnBlockStore, before any decoded object reuse.
	_, _, err := s.ensurePointer(ctx)
	return err
}

func (s *CdnBlockStore) setPointer(ctx context.Context, ptr *cdn.CdnRootPointer) uint64 {
	var epoch uint64
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if s.memCache != nil {
			s.memCache.reset()
		}

		// Pointer, manifest, and decoded-cache epoch publish under one bcast lock.
		// Reads take the same lock while snapshotting the manifest so they cannot
		// pair an old pointer decision with a new manifest view.
		s.invalidateDecodedBlocks(ctx)
		if ptr == nil {
			s.pfs.UpdateManifest(nil)
		} else {
			s.pfs.UpdateManifest(ptr.GetPacks())
		}
		s.pointer = ptr
		s.pointerTime = time.Now()
		s.pointerEpoch++
		epoch = s.pointerEpoch
		broadcastFn()
	})
	return epoch
}

// ensurePointer returns the cached pointer if fresh, otherwise refreshes.
// Returns nil, nil if the CDN Space has no content.
func (s *CdnBlockStore) ensurePointer(ctx context.Context) (*cdn.CdnRootPointer, uint64, error) {
	ttl := s.opts.PointerTTL
	if ttl == 0 {
		ttl = DefaultPointerTTL
	}

	var cached *cdn.CdnRootPointer
	var fetchedAt time.Time
	var epoch uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cached = s.pointer
		fetchedAt = s.pointerTime
		epoch = s.pointerEpoch
	})
	if !fetchedAt.IsZero() && (ttl < 0 || time.Since(fetchedAt) < ttl) {
		return cached, epoch, nil
	}
	ptr, err := FetchRootPointer(ctx, s.cli, s.opts.CdnBaseURL, s.opts.SpaceID)
	if err != nil {
		return nil, 0, err
	}
	epoch = s.setPointer(ctx, ptr)
	return ptr, epoch, nil
}

func (s *CdnBlockStore) withCurrentManifest(ctx context.Context, read func() error) error {
	for {
		_, epoch, err := s.ensurePointer(ctx)
		if err != nil {
			return err
		}
		err = read()
		stale := false
		s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			stale = s.pointerEpoch != epoch
		})
		if !stale {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (s *CdnBlockStore) getCurrentCachedBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.getCachedBlock(ctx, ref)
	if err != nil || !found {
		return data, found, err
	}
	var exists bool
	err = s.withCurrentManifest(ctx, func() error {
		var err error
		exists, err = s.pfs.GetBlockExists(ctx, ref)
		return err
	})
	if err != nil || !exists {
		return nil, false, err
	}
	return data, true, nil
}

func (s *CdnBlockStore) getCachedBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	target := s.getWritebackTarget()
	if target == nil {
		return nil, false, nil
	}
	return target.GetBlock(ctx, ref)
}

func (s *CdnBlockStore) invalidateDecodedBlocks(ctx context.Context) {
	if s.decodedBlocks == nil {
		return
	}

	// A CDN pointer swap replaces the manifest under the store. Decoded hits
	// must be equivalent to reading through the current manifest, not an older
	// CDN root that happened to decode the same ref earlier.
	s.decodedBlocks.InvalidateAll(ctx)
}

func (s *CdnBlockStore) getWritebackTarget() block.StoreOps {
	var target block.StoreOps
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		target = s.writebackTarget
	})
	return target
}

// _ is a type assertion
var _ block.StoreOps = (*CdnBlockStore)(nil)

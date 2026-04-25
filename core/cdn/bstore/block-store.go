package cdn_bstore

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"

	"github.com/s4wave/spacewave/core/cdn"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
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
}

// CdnBlockStore is a read-only block.StoreOps backed by the public Spacewave
// CDN. Reads are served by a packfile_store.PackfileStore fed by an anonymous
// HTTP Range opener. Writes return ErrReadOnly. The cached root pointer is
// refreshed lazily when the TTL expires or Invalidate is called.
type CdnBlockStore struct {
	opts   Options
	cli    *http.Client
	opener packfile_store.Opener
	cache  *memIndexCache
	pfs    *packfile_store.PackfileStore

	bcast       broadcast.Broadcast
	pointer     *cdn.CdnRootPointer
	pointerTime time.Time
}

// NewCdnBlockStore constructs a new CdnBlockStore. The pointer is fetched
// lazily on the first read; pass a pre-populated pointer via SetPointer if
// the caller already has one.
func NewCdnBlockStore(opts Options) (*CdnBlockStore, error) {
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
	cache := newMemIndexCache()
	opener := NewAnonymousOpener(cli, opts.CdnBaseURL, opts.SpaceID)
	pfs := packfile_store.NewPackfileStore(opener, cache)
	return &CdnBlockStore{
		opts:   opts,
		cli:    cli,
		opener: opener,
		cache:  cache,
		pfs:    pfs,
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

// GetBlock reads a block by reference, refreshing the pointer if needed.
func (s *CdnBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if _, err := s.ensurePointer(ctx); err != nil {
		return nil, false, err
	}
	return s.pfs.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (s *CdnBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if _, err := s.ensurePointer(ctx); err != nil {
		return false, err
	}
	return s.pfs.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks whether each block exists.
func (s *CdnBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if _, err := s.ensurePointer(ctx); err != nil {
		return nil, err
	}
	return s.pfs.GetBlockExistsBatch(ctx, refs)
}

// StatBlock returns block metadata without reading the data.
func (s *CdnBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if _, err := s.ensurePointer(ctx); err != nil {
		return nil, err
	}
	return s.pfs.StatBlock(ctx, ref)
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

// PutBlockBackground is not supported on an anonymous CDN block store.
func (s *CdnBlockStore) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// RmBlock is not supported on an anonymous CDN block store.
func (s *CdnBlockStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Flush has no buffered work for the anonymous CDN block store.
func (s *CdnBlockStore) Flush(_ context.Context) error {
	return nil
}

// BeginDeferFlush is a no-op for the anonymous CDN block store.
func (s *CdnBlockStore) BeginDeferFlush() {}

// EndDeferFlush is a no-op for the anonymous CDN block store.
func (s *CdnBlockStore) EndDeferFlush(_ context.Context) error {
	return nil
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
	ptr, err := FetchRootPointer(ctx, s.cli, s.opts.CdnBaseURL, s.opts.SpaceID)
	if err != nil {
		return nil, err
	}
	s.setPointer(ptr)
	return ptr, nil
}

// Invalidate drops the cached pointer so the next read re-fetches.
func (s *CdnBlockStore) Invalidate() {
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		s.pointer = nil
		s.pointerTime = time.Time{}
		broadcastFn()
	})
	s.cache.reset()
	s.pfs.UpdateManifest(nil)
}

// SetPointer replaces the cached pointer without issuing a network request.
// Used by callers that receive a pointer via an external channel (for example
// the cdn-root-changed session WS frame which will land in Phase F).
func (s *CdnBlockStore) SetPointer(ptr *cdn.CdnRootPointer) {
	s.setPointer(ptr)
}

func (s *CdnBlockStore) setPointer(ptr *cdn.CdnRootPointer) {
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		s.pointer = ptr
		s.pointerTime = time.Now()
		broadcastFn()
	})
	s.cache.reset()
	if ptr == nil {
		s.pfs.UpdateManifest(nil)
		return
	}
	s.pfs.UpdateManifest(ptr.GetPacks())
}

// ensurePointer returns the cached pointer if fresh, otherwise refreshes.
// Returns nil, nil if the CDN Space has no content.
func (s *CdnBlockStore) ensurePointer(ctx context.Context) (*cdn.CdnRootPointer, error) {
	ttl := s.opts.PointerTTL
	if ttl == 0 {
		ttl = DefaultPointerTTL
	}

	var cached *cdn.CdnRootPointer
	var fetchedAt time.Time
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cached = s.pointer
		fetchedAt = s.pointerTime
	})
	if cached != nil && (ttl < 0 || time.Since(fetchedAt) < ttl) {
		return cached, nil
	}
	return s.Refresh(ctx)
}

// _ is a type assertion
var _ block.StoreOps = (*CdnBlockStore)(nil)

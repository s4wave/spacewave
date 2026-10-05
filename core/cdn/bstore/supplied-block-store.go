package cdn_bstore

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/cdn"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_hash "github.com/s4wave/spacewave/net/hash"
)

// SuppliedOptions configure a SuppliedBlockStore.
type SuppliedOptions struct {
	// CdnBaseURL is the public CDN origin (e.g. https://cdn.spacewave.app).
	CdnBaseURL string
	// RootPointerBaseURL optionally serves root.packedmsg from another origin.
	// Empty reads the pointer from CdnBaseURL.
	RootPointerBaseURL string
	// SpaceID is the CDN Space ULID.
	SpaceID string
	// HttpClient overrides the default http.Client for root pointer fetches.
	HttpClient *http.Client
	// PointerTTL is the age at which a read re-fetches the root pointer. Zero
	// falls back to DefaultPointerTTL; negative disables the re-fetch.
	PointerTTL time.Duration
	// Store serves every block read. Another owner holds the CDN transport,
	// pack readers, and durable writeback behind it.
	Store block_store.Store
}

// SuppliedBlockStore reads a CDN Space through a block store supplied by
// another owner while fetching the root pointer itself.
//
// It exists for compositions that mount one CDN Space in two processes: the
// owning process holds the single CDN transport, pack readers, index cache,
// decoded-block cache, and durable writeback, and every other process reads
// the same Space through that owner rather than opening a second transport.
// The root pointer stays local so the reader can build its world view. The
// owner replaces its pack manifest as its own pointer ages, so reads here
// re-fetch an aged pointer too and the world view follows the owner's
// manifest.
type SuppliedBlockStore struct {
	// store serves every block read.
	store block_store.Store
	// cli fetches the root pointer.
	cli *http.Client
	// rootBaseURL is the origin serving root.packedmsg.
	rootBaseURL string
	// spaceID is the CDN Space ULID.
	spaceID string
	// pointerTTL is the age at which a read re-fetches the pointer.
	pointerTTL time.Duration

	// bcast guards the cached pointer and its fetch state.
	bcast broadcast.Broadcast
	// pointer is the most recently fetched root pointer.
	pointer *cdn.CdnRootPointer
	// pointerTime is when pointer was fetched; zero before the first Refresh.
	pointerTime time.Time
	// refreshing is set while a read re-fetches an aged pointer.
	refreshing bool
}

// NewSuppliedBlockStore constructs a SuppliedBlockStore. The pointer is not
// fetched until Refresh is called; reads re-fetch it once it is older than
// PointerTTL.
func NewSuppliedBlockStore(opts SuppliedOptions) (*SuppliedBlockStore, error) {
	// Require the CDN origin, Space, and supplied store.
	if opts.CdnBaseURL == "" {
		return nil, errors.New("cdn bstore: CdnBaseURL required")
	}
	if opts.SpaceID == "" {
		return nil, errors.New("cdn bstore: SpaceID required")
	}
	if opts.Store == nil {
		return nil, errors.New("cdn bstore: Store required")
	}

	// Resolve the HTTP client, defaulting to the shared client.
	cli := opts.HttpClient
	if cli == nil {
		cli = http.DefaultClient
	}
	ttl := opts.PointerTTL
	if ttl == 0 {
		ttl = DefaultPointerTTL
	}
	return &SuppliedBlockStore{
		store:       opts.Store,
		cli:         cli,
		rootBaseURL: RootPointerBaseURL(opts.CdnBaseURL, opts.RootPointerBaseURL),
		spaceID:     opts.SpaceID,
		pointerTTL:  ttl,
	}, nil
}

// GetID returns the block store id; CDN block stores use the Space ULID
// verbatim because the mount is 1:1 with a Space.
func (s *SuppliedBlockStore) GetID() string {
	return s.spaceID
}

// GetDecodedBlockCache returns nil: the supplying owner holds the decoded-block
// cache for this Space, so readers build their own short-lived cache instead of
// borrowing one across the supply boundary.
func (s *SuppliedBlockStore) GetDecodedBlockCache() *block.DecodedBlockCache {
	return nil
}

// ReclaimStorage returns zero: the supplying owner holds the storage.
func (s *SuppliedBlockStore) ReclaimStorage(context.Context, bstore.ReclaimFence) (time.Time, error) {
	return time.Time{}, nil
}

// Pointer returns the currently-cached root pointer without triggering a
// refresh. Returns nil if no pointer has been fetched yet.
func (s *SuppliedBlockStore) Pointer() *cdn.CdnRootPointer {
	// Read the cached pointer under the broadcast lock.
	var ptr *cdn.CdnRootPointer
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ptr = s.pointer
	})
	return ptr
}

// Refresh forces a re-fetch of the root pointer.
// Returns the new pointer (nil if the CDN Space is empty).
func (s *SuppliedBlockStore) Refresh(ctx context.Context) (*cdn.CdnRootPointer, error) {
	// Fetch the root pointer from the CDN.
	ptr, err := FetchRootPointer(ctx, s.cli, s.rootBaseURL, s.spaceID)
	if err != nil {
		return nil, err
	}
	// Publish a changed pointer to watchers.
	s.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		changed := !ptr.EqualVT(s.pointer)
		s.pointer = ptr
		s.pointerTime = time.Now()
		if changed {
			broadcastFn()
		}
	})
	return ptr, nil
}

// WaitPointer blocks until the cached root pointer differs from prev and
// returns it.
func (s *SuppliedBlockStore) WaitPointer(ctx context.Context, prev *cdn.CdnRootPointer) (*cdn.CdnRootPointer, error) {
	return waitPointerChange(ctx, &s.bcast, prev, func() (*cdn.CdnRootPointer, bool) {
		return s.pointer, false
	})
}

// refreshAgedPointer re-fetches a pointer older than the pointer TTL. One read
// fetches at a time while the others continue. A failed fetch keeps the
// current pointer for the next read to retry: the supplying owner serves the
// block either way, so the fetch never fails the read.
func (s *SuppliedBlockStore) refreshAgedPointer(ctx context.Context) {
	// Claim the re-fetch of a pointer past its TTL.
	var due bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		due = !s.refreshing && !s.pointerTime.IsZero() &&
			s.pointerTTL >= 0 && time.Since(s.pointerTime) >= s.pointerTTL
		if due {
			s.refreshing = true
		}
	})
	if !due {
		return
	}

	// Re-fetch the pointer, then release the claim.
	_, _ = s.Refresh(ctx)
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.refreshing = false
	})
}

// Close releases resources used by the store. The supplied store is owned by
// its supplier and is not closed here.
func (s *SuppliedBlockStore) Close() {}

// GetHashType forwards to the supplied store.
func (s *SuppliedBlockStore) GetHashType() block_hash.HashType {
	return s.store.GetHashType()
}

// GetSupportedFeatures forwards to the supplied store.
func (s *SuppliedBlockStore) GetSupportedFeatures() block.StoreFeature {
	return s.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the supplied store.
func (s *SuppliedBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	s.refreshAgedPointer(ctx)
	return s.store.BeginReadOperation(ctx)
}

// GetBlock forwards to the supplied store.
func (s *SuppliedBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.refreshAgedPointer(ctx)
	return s.store.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (s *SuppliedBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	s.refreshAgedPointer(ctx)
	return s.store.GetStoredBlock(ctx, ref)
}

// GetBlockExists forwards to the supplied store.
func (s *SuppliedBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	s.refreshAgedPointer(ctx)
	return s.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards to the supplied store.
func (s *SuppliedBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	s.refreshAgedPointer(ctx)
	return s.store.GetBlockExistsBatch(ctx, refs)
}

// StatBlock forwards to the supplied store.
func (s *SuppliedBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	s.refreshAgedPointer(ctx)
	return s.store.StatBlock(ctx, ref)
}

// PutBlock is not supported: the supplying owner writes this Space.
func (s *SuppliedBlockStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch is not supported: the supplying owner writes this Space.
func (s *SuppliedBlockStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return block_store.ErrReadOnly
}

// RmBlock is not supported: the supplying owner writes this Space.
func (s *SuppliedBlockStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync forwards the durability barrier to the supplied store.
func (s *SuppliedBlockStore) Sync(ctx context.Context) (bool, error) {
	return s.store.Sync(ctx)
}

// _ is a type assertion
var (
	_ block.StoreOps = (*SuppliedBlockStore)(nil)
	_ RootBlockStore = (*SuppliedBlockStore)(nil)
)

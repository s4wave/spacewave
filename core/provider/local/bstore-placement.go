package provider_local

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"golang.org/x/sync/errgroup"
)

// settingsWatch follows the account settings.
type settingsWatch struct {
	// states is the account settings state.
	states ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
	// snapshot is the last settings snapshot read.
	snapshot sobject.SharedObjectStateSnapshot
	// release releases the settings mount.
	release func()
}

// lookupAccountSettingsRef returns the account settings SharedObject, or nil
// when the account has no settings yet.
func (a *ProviderAccount) lookupAccountSettingsRef(ctx context.Context) (*sobject.SharedObjectRef, error) {
	ref, err := a.GetAccountSettingsRef(ctx)
	if errors.Is(err, sobject.ErrSharedObjectNotFound) {
		return nil, nil
	}
	return ref, err
}

// watchAccountSettings mounts the account settings for watching.
func (a *ProviderAccount) watchAccountSettings(ctx context.Context, ref *sobject.SharedObjectRef) (*settingsWatch, error) {
	// Mount the settings object and open its state for watching.
	so, releaseSo, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	states, releaseStates, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		releaseSo()
		return nil, err
	}
	return &settingsWatch{
		states: states,
		release: func() {
			releaseStates()
			releaseSo()
		},
	}, nil
}

// next waits for the next settings change and returns the settings.
func (w *settingsWatch) next(ctx context.Context) (*account_settings.AccountSettings, error) {
	// Wait for the next settings change and decode it.
	snapshot, err := w.states.WaitValueChange(ctx, w.snapshot, nil)
	if err != nil {
		return nil, err
	}
	w.snapshot = snapshot
	settings, err := account_settings.ReadSnapshot(ctx, snapshot)
	return settings, err
}

// nextBackend waits for the next settings change and returns the store's
// placement, the storage backend holding it and the SQL backend its reads fall
// back to, or nil for all three when the store is on the account's own
// storage. The fallback is nil when the placement names none.
func (t *bstoreTracker) nextBackend(
	ctx context.Context,
	watch *settingsWatch,
) (*account_settings.BlockStorePlacement, *account_settings.StorageBackend, *account_settings.StorageBackend, error) {
	// Read the next settings change and resolve this store's backend.
	settings, err := watch.next(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Resolve the placement and its storage backend for this store.
	placement := settings.FindBlockStorePlacement(t.id)
	if placement == nil {
		return nil, nil, nil, nil
	}
	backend := settings.FindStorageBackend(placement.GetStorageBackendId())
	if backend == nil {
		return nil, nil, nil, errors.Wrap(account_settings.ErrStorageBackendNotFound, placement.GetStorageBackendId())
	}

	// Resolve the read fallback, which the settings validate as an SQL backend.
	fallbackID := placement.GetFallbackStorageBackendId()
	if fallbackID == "" {
		return placement, backend, nil, nil
	}
	fallback := settings.FindStorageBackend(fallbackID)
	if fallback == nil {
		return nil, nil, nil, errors.Wrap(account_settings.ErrStorageBackendNotFound, fallbackID)
	}
	return placement, backend, fallback, nil
}

// uploadPolicy returns when a placement's cache settings upload its writes and
// how many bytes the local disk keeps. A low mark of zero or past the high
// mark is nine tenths of the high mark.
func uploadPolicy(cache *account_settings.BlockStoreCache) block_store_writeback.Policy {
	high := min(cache.GetResidentHighBytes(), math.MaxInt64)
	low := cache.GetResidentLowBytes()
	if low == 0 || low > high {
		low = high / 10 * 9
	}
	return block_store_writeback.Policy{
		Settle:          time.Duration(cache.GetSettleSeconds()) * time.Second,
		MaxPendingBytes: int64(min(cache.GetMaxPendingBytes(), math.MaxInt64)),
		ResidentHigh:    int64(high),
		ResidentLow:     int64(low),
		MaxUploadReads:  int(cache.GetMaxUploadReads()),
		UploadReadSlow:  time.Duration(cache.GetUploadReadSlowMillis()) * time.Millisecond,
		FillQueueBytes:  int64(min(cache.GetFillQueueBytes(), math.MaxInt64)),
	}
}

// runPlacedUploads keeps every placed block store mounted while the account
// runs, so writes queued in a closed Space still upload, and deletes the
// objects of the block stores that left a backend.
func (a *ProviderAccount) runPlacedUploads(ctx context.Context) error {
	// Watch the account settings for placement changes.
	ref, err := a.lookupAccountSettingsRef(ctx)
	if err != nil || ref == nil {
		return err
	}
	watch, err := a.watchAccountSettings(ctx, ref)
	if err != nil {
		return err
	}
	defer watch.release()

	// Hold a reference on every placed block store while the account runs.
	releases := a.newStorageReleases()
	releases.SetContext(ctx, true)
	defer releases.SetContext(nil, false)

	// Track the stores this replica holds by block store ID.
	held := make(map[string]*keyed.KeyedRef[string, *bstoreTracker])
	defer func() {
		for _, ref := range held {
			ref.Release()
		}
	}()

	// Sync pending releases with the settings' pending placements.
	for {
		settings, err := watch.next(ctx)
		if err != nil {
			return err
		}

		releases.SyncKeys(pendingStorageReleases(settings), false)

		// Release stores that left a backend and hold newly placed stores.
		placed := make(map[string]struct{}, len(settings.GetBlockStorePlacements()))
		for _, placement := range settings.GetBlockStorePlacements() {
			placed[placement.GetBlockStoreId()] = struct{}{}
		}
		for id, ref := range held {
			if _, ok := placed[id]; !ok {
				ref.Release()
				delete(held, id)
			}
		}
		for id := range placed {
			if held[id] == nil {
				held[id], _, _ = a.bstores.AddKeyRef(id)
			}
		}
	}
}

// backendStore is a block store on a storage backend's bucket.
type backendStore interface {
	block.StoreOps
	// Reclaim drops the blocks live reports dead from the bucket, calling
	// fence after listing the bucket and before dropping any. Returns the
	// time a pass next comes due without further writes, or zero.
	Reclaim(
		ctx context.Context,
		fence func(context.Context) error,
		live func(context.Context, []*block.BlockRef) ([]bool, error),
	) (time.Time, error)
	// Close releases the store's open readers.
	Close()
}

// preloader is a backend store that can read its packfile entries before its
// first read.
type preloader interface {
	// Preload starts loading the descriptors needed for later lookups.
	Preload(ctx context.Context) error
}

// openBackend is the open store of a storage backend.
type openBackend struct {
	// backend is the storage backend the store reads and writes.
	backend *account_settings.StorageBackend
	// store is the block store on the backend's bucket.
	store backendStore
}

// runUpload uploads the store's queued blocks, and the finalized packs drain
// holds when it is not nil, to the backend's bucket, which also serves reads
// the local store misses.
//
// The bucket store stays open across failed uploads and their retries, so an
// unreachable bucket fails reads with its own error and the store keeps the
// packfiles it knows. Clearing or changing the placement cancels ctx and
// closes the store.
func (t *bstoreTracker) runUpload(
	ctx context.Context,
	wb *block_store_writeback.Store,
	drain *packahead.Drain,
	backend *account_settings.StorageBackend,
) error {
	// Reopen the backend store when the placement changed.
	remote := t.remote.Load()
	if remote == nil || !remote.backend.EqualVT(backend) {
		store, err := t.openBackendStore(ctx, backend)
		if err != nil {
			wb.SetError(err)
			return err
		}
		remote = &openBackend{backend: backend, store: store}
		t.swapRemote(remote)

		// Read the store's packfile entries now, so the first read it serves
		// after a start, often a commit holding the World writer, does not
		// wait behind a cold read of them. A failed load retries on that read.
		if pre, ok := store.(preloader); ok {
			go func() {
				if err := pre.Preload(ctx); err != nil && ctx.Err() == nil {
					t.a.le.WithError(err).Warn("unable to preload the backend's packfile entries")
				}
			}()
		}
	}

	// Upload the queued blocks and the finalized packs, both paced by the
	// account's admit, closing the store when the context ends. A backend
	// that takes no packfiles leaves the packs local.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return wb.UploadPaced(egCtx, remote.store, t.admitUpload)
	})
	if drain != nil {
		if packRemote, ok := remote.store.(packahead.Remote); ok {
			eg.Go(func() error {
				t.runDrain(egCtx, drain, packRemote)
				return nil
			})
		} else {
			t.a.le.WithField("bstore-id", t.id).Warn("storage backend takes no packfiles, keeping the packahead log local")
		}
	}
	err := eg.Wait()
	if ctx.Err() != nil {
		t.swapRemote(nil)
	}
	return err
}

// drainRetryDelay is how long a failed pack drain waits before it retries.
const drainRetryDelay = 5 * time.Second

// runDrain drains the store's packahead log to remote until ctx ends,
// retrying after a failed step, so a pack the backend refuses never stops
// the queued block upload beside it.
func (t *bstoreTracker) runDrain(ctx context.Context, drain *packahead.Drain, remote packahead.Remote) {
	for {
		err := drain.Run(ctx, remote)
		if ctx.Err() != nil {
			return
		}
		t.a.le.WithError(err).WithField("bstore-id", t.id).Warn("packahead drain failed, retrying")
		select {
		case <-ctx.Done():
			return
		case <-time.After(drainRetryDelay):
		}
	}
}

// openBackendStore opens the backend's bucket with its credential Secret.
func (t *bstoreTracker) openBackendStore(
	ctx context.Context,
	backend *account_settings.StorageBackend,
) (backendStore, error) {
	// An FTPS server logs in from its netrc file and has no credential Secret.
	le := t.a.le.WithField("bstore-id", t.id)
	if location := backend.GetFtps(); location != nil {
		return buildFTPSBlockStore(le, location, nil, t.packIndexStore(), t.nodeReads())
	}

	// Open the backend's bucket with its credential Secret.
	creds, err := t.a.ReadStorageCredentials(ctx, backend)
	if err != nil {
		return nil, err
	}
	if location := backend.GetSql(); location != nil {
		return buildSQLBlockStore(ctx, le, location, t.id, creds)
	}
	return buildS3BlockStore(le, backend.GetS3(), t.id, creds, t.packIndexStore())
}

// openFallbackStore opens the SQL backend the store reads after its own
// backend misses, with the backend's credential Secret. The store is read only
// to its callers and connects on its first read.
func (t *bstoreTracker) openFallbackStore(
	ctx context.Context,
	backend *account_settings.StorageBackend,
) (backendStore, error) {
	// Open the fallback's table with its credential Secret.
	creds, err := t.a.ReadStorageCredentials(ctx, backend)
	if err != nil {
		return nil, err
	}
	le := t.a.le.WithField("bstore-id", t.id).WithField("fallback", backend.GetId())
	return buildSQLFallbackStore(le, backend.GetSql(), t.id, creds)
}

// swapRemote replaces the open backend store and closes the previous one.
func (t *bstoreTracker) swapRemote(next *openBackend) {
	// Publish the handle and wake the waiters before releasing the old store.
	var prev *openBackend
	t.remoteBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		prev = t.remote.Swap(next)
		if prev != next {
			broadcast()
		}
	})

	// Closing a backend may block or call back into the tracker.
	if prev != nil && prev != next {
		prev.store.Close()
	}
}

// SetSQLPrimary makes the account's block stores placed on an SQL backend,
// as mounted after the call, write to that backend and read the local bucket,
// read only, before it. It also stops the account's GC cleanup
// and backend reclaim, since the backend's blocks reference local blocks
// without graph edges.
func (a *ProviderAccount) SetSQLPrimary(primary bool) {
	a.sqlPrimary.Store(primary)
}

// sqlMaxConns is the pool size of the SQL backend stores opened after it is
// set, zero for the store's default.
var sqlMaxConns atomic.Int32

// SetSQLMaxConns bounds the open connections of each SQL backend store opened
// after the call; zero or less uses the store's default.
func SetSQLMaxConns(n int) {
	sqlMaxConns.Store(int32(max(n, 0))) // #nosec G115 -- pool sizes fit in int32.
}

// packIndexStores maps the storage volume ID of an account to the store its
// pack indexes live in, as SetPackIndexStore registered it.
var packIndexStores sync.Map

// SetPackIndexStore keeps the pack indexes of the FTPS and S3 placed block
// stores of the account providerID/accountID in store instead of the account
// volume. It must be called before the account starts, since the account
// mounts its placed block stores as it starts. Each index keeps its block
// store's key prefix, so the keys match the ones the volume would hold and a
// copy moves them unchanged. A nil store returns the account to its volume.
// The caller keeps store open while the account runs.
func SetPackIndexStore(providerID, accountID string, store kvtx.Store) {
	id := StorageVolumeID(providerID, accountID)
	if store == nil {
		packIndexStores.Delete(id)
		return
	}
	packIndexStores.Store(id, store)
}

// packIndexStore returns the key space for the pack index of the block store's
// packs: the account's registered pack index store when it has one, else the
// account volume's, or nil when the volume has no key/value store.
func (t *bstoreTracker) packIndexStore() kvtx.Store {
	prefix := block_store_s3.PackIndexPrefix(t.id)
	id := StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID())
	if store, ok := packIndexStores.Load(id); ok {
		return kvtx_prefixer.NewPrefixer(store.(kvtx.Store), prefix)
	}
	kvVol, ok := t.a.vol.(kvtx_volume.KvtxVolume)
	if !ok {
		return nil
	}
	return kvtx_prefixer.NewPrefixer(kvVol.GetKvtxStore(), prefix)
}

package provider_local

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// ErrSweepServeOnly is returned by the sweep's pack calls for a store in serve
// mode: another process writes its index, so the sweep runs there.
var ErrSweepServeOnly = errors.New("store is in serve mode: the pack sweep runs on the process that writes its index")

// storeSweep is the pack sweep's view of one block store of an account: the
// write observer its operator set, and the write-back of the mount that
// applies it. It outlives every mount of the store, so an observer set once
// reaches each later mount, and a remount never opens a gap in the write log
// that the sweep could not see.
type storeSweep struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// observer is the write observer, nil for none.
	observer block_store_writeback.WriteObserver
	// wb is the mounted write-back, nil while the store is not mounted.
	wb *block_store_writeback.Store
	// roots is the ownership graph of the volume holding the mounted store,
	// nil while the store is not mounted.
	roots block_gc.RefGraphOps
	// bucket is the mounted store's bucket in that volume.
	bucket string
	// le logs for the mounted store, nil while it is not mounted.
	le *logrus.Entry
}

// storeSweeps maps each block store to its storeSweep. Entries are never
// removed: one small value per store the process mounted or named.
var storeSweeps sync.Map

// lookupSweep returns the sweep state of a block store, adding it.
func lookupSweep(key adoptKey) *storeSweep {
	value, _ := storeSweeps.LoadOrStore(key, &storeSweep{})
	return value.(*storeSweep)
}

// mountSweep applies the store's write observer to the mount's write-back,
// before the mount takes any write, and returns the function that detaches
// it at unmount.
func (t *bstoreTracker) mountSweep(wb *block_store_writeback.Store, bucketID string) func() {
	// Record the mount with its bucket's graph, and give it the store's
	// observer.
	s := lookupSweep(adoptKey{volumeID: StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()), storeID: t.id})
	s.mtx.Lock()
	s.wb, s.roots, s.bucket, s.le = wb, t.a.vol.GetRefGraph(), bucketID, t.a.le
	wb.SetObserver(s.observer)
	s.mtx.Unlock()

	// Forget the mount at unmount, unless a later mount replaced it.
	return func() {
		s.mtx.Lock()
		if s.wb == wb {
			s.wb, s.roots, s.bucket, s.le = nil, nil, "", nil
		}
		s.mtx.Unlock()
	}
}

// SetWriteObserver sets the observer of every block the block store storeID
// of the account accepts or fills, nil for none, on the mount open now and on
// each later one. A pack sweep's write log is the observer: it must see every
// write from the moment it starts, or it cannot prove a pack unreached.
func SetWriteObserver(providerID, accountID, storeID string, o block_store_writeback.WriteObserver) {
	// Keep the observer for later mounts and apply it to the open one.
	s := lookupSweep(adoptKey{volumeID: StorageVolumeID(providerID, accountID), storeID: storeID})
	s.mtx.Lock()
	defer s.mtx.Unlock()
	prev := s.observer
	s.observer = o
	if s.wb != nil {
		s.wb.SetObserver(o)
		return
	}
	block_store_writeback.TellAttached(prev, o)
}

// OldestOperations returns when the oldest write and read operation still open
// on the mounted block store storeID began, each zero for none, as
// block_store_writeback.MarkingStore.OldestOperations reports them. ok is
// false while the store is not mounted: its operations are then unknown, not
// none, since a remount may be opening them.
func OldestOperations(providerID, accountID, storeID string) (write, read time.Time, ok bool) {
	// Find the open mount.
	s := lookupSweep(adoptKey{volumeID: StorageVolumeID(providerID, accountID), storeID: storeID})
	s.mtx.Lock()
	wb := s.wb
	s.mtx.Unlock()
	if wb == nil {
		return time.Time{}, time.Time{}, false
	}

	// Report its oldest open operations.
	ops := wb.OldestOperations()
	return ops.Write, ops.Read, true
}

// SweepRoots lists the roots of the mounted block store storeID of the
// account for a pack sweep, from the ownership graph of the volume that holds
// it, as block_gc.OwnedRoots finds them: every block the store's bucket holds
// through a named root or a write no parent references yet, and every block a
// reader of the volume pins or stages. The bucket's named roots are every
// root its writers set through block.SetRetainedRoots, whatever its name, so
// a kind of root added later is listed without a change here. Another
// bucket's roots are left out. A store not mounted, a volume with no graph, a
// store that does not record its roots there (block.SupportsRootRetention
// false, as over a packahead log or a volume without atomic publication), or
// a graph read that fails ends the listing with an error, so the sweep keeps
// every pack.
func SweepRoots(providerID, accountID, storeID string) block_packsweep_admin.RootLister {
	return &sweepRoots{key: adoptKey{volumeID: StorageVolumeID(providerID, accountID), storeID: storeID}}
}

// sweepRoots lists a mounted store's roots from its volume's graph.
type sweepRoots struct {
	key adoptKey
}

// ListRoots calls fn with each root whose name starts with prefix, named by
// the graph node that owns it.
func (r *sweepRoots) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	// Read the mounted store's graph and bucket.
	s := lookupSweep(r.key)
	s.mtx.Lock()
	wb, rg, bucket, le := s.wb, s.roots, s.bucket, s.le
	s.mtx.Unlock()
	if wb == nil || rg == nil || bucket == "" {
		return errors.New("the block store is not mounted on a volume with an ownership graph")
	}

	// Refuse a store that does not record its roots in the graph: its writers'
	// named roots and pins never reach it, so the walk would miss them silently.
	if !block.SupportsRootRetention(wb) {
		return errors.New("the block store does not record its roots in the ownership graph")
	}

	// Walk the owners, leaving out the other buckets.
	own := block_gc.BucketIRI(bucket)
	skip := func(node string) bool {
		_, isBucket := block_gc.ParseBucketIRI(node)
		return isBucket && node != own
	}
	start, listed := time.Now(), 0
	frontier, err := block_gc.OwnedRoots(ctx, rg, skip, func(owner string, ref *block.BlockRef) error {
		name := owner + "/" + ref.MarshalString()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}
		listed++
		return fn(block.NamedRoot{Name: name, Ref: ref})
	})
	if err != nil {
		return err
	}

	// Log the walk once per listing: its frontier is what it held in memory.
	le.WithFields(logrus.Fields{"bucket": bucket, "frontier": frontier, "roots": listed, "took": time.Since(start).String()}).Info("listed the owned roots")
	return nil
}

// SweepPacks returns the pack states of the mounted block store storeID of the
// account, for a pack sweep's admin server. Each call resolves the store's
// open backend, as AdoptBucketPack does, so a remount or a swapped backend is
// followed, and fails while none is open or the store serves another
// process's index.
func SweepPacks(providerID, accountID, storeID string) block_packsweep_admin.PackStates {
	return &sweepPacks{providerID: providerID, accountID: accountID, storeID: storeID}
}

// sweepPacks forwards the pack sweep's calls to a mounted store's backend.
type sweepPacks struct {
	providerID, accountID, storeID string
}

// packs returns the open backend's pack states.
func (s *sweepPacks) packs() (block_packsweep_admin.PackStates, error) {
	// Find the mounted store, refusing one that serves another's index.
	a, err := lookupAdopter(s.providerID, s.accountID, s.storeID)
	if err != nil {
		return nil, err
	}
	if a.p.serveOnly {
		return nil, ErrSweepServeOnly
	}

	// Resolve its open backend's pack states.
	remote, _ := a.t.readBackend(a.p)
	if remote == nil {
		return nil, errors.New("storage backend not open")
	}
	packs, ok := remote.(block_packsweep_admin.PackStates)
	if !ok {
		return nil, errors.New("storage backend has no packs to sweep")
	}
	return packs, nil
}

// Holds returns the sweep state of packs ids, or of every moved pack.
func (s *sweepPacks) Holds(ctx context.Context, ids []string) ([]block_store_s3.PackHold, error) {
	p, err := s.packs()
	if err != nil {
		return nil, err
	}
	return p.Holds(ctx, ids)
}

// LocatePacks returns the registered pack the index places each block in.
func (s *sweepPacks) LocatePacks(ctx context.Context, refs []*block.BlockRef) ([]string, error) {
	p, err := s.packs()
	if err != nil {
		return nil, err
	}
	return p.LocatePacks(ctx, refs)
}

// Quarantine holds registered packs out of the reads.
func (s *sweepPacks) Quarantine(ctx context.Context, ids []string) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.Quarantine(ctx, ids)
}

// Lift returns quarantined packs to the reads.
func (s *sweepPacks) Lift(ctx context.Context, ids []string) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.Lift(ctx, ids)
}

// BeginTrash moves quarantined packs to the trashing state.
func (s *sweepPacks) BeginTrash(ctx context.Context, ids []string) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.BeginTrash(ctx, ids)
}

// Remove repairs the h rows of keys and drops the trashing packs ids.
func (s *sweepPacks) Remove(ctx context.Context, ids []string, keys []block_store_s3.KeyHolders) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.Remove(ctx, ids, keys)
}

// RepairRemoved repairs the h rows of keys and leaves the packs trashing.
func (s *sweepPacks) RepairRemoved(ctx context.Context, ids []string, keys []block_store_s3.KeyHolders) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.RepairRemoved(ctx, ids, keys)
}

// Restore registers again trashing packs whose files are back.
func (s *sweepPacks) Restore(ctx context.Context, ids []string) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.Restore(ctx, ids)
}

// AdoptPack registers a pack another writer uploaded.
func (s *sweepPacks) AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error {
	p, err := s.packs()
	if err != nil {
		return err
	}
	return p.AdoptPack(ctx, entry, keys)
}

// _ are type assertions: a bucket store's open backend is the pack store.
var (
	_ block_packsweep_admin.PackStates = (*sweepPacks)(nil)
	_ block_packsweep_admin.PackStates = (*block_store_s3.PackStore)(nil)
)

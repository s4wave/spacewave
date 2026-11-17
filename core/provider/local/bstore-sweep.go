package provider_local

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
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
func (t *bstoreTracker) mountSweep(wb *block_store_writeback.Store) func() {
	// Record the mount and give it the store's observer.
	s := lookupSweep(adoptKey{volumeID: StorageVolumeID(t.a.GetProviderID(), t.a.GetAccountID()), storeID: t.id})
	s.mtx.Lock()
	s.wb = wb
	wb.SetObserver(s.observer)
	s.mtx.Unlock()

	// Forget the mount at unmount, unless a later mount replaced it.
	return func() {
		s.mtx.Lock()
		if s.wb == wb {
			s.wb = nil
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
	s.observer = o
	if s.wb != nil {
		s.wb.SetObserver(o)
	}
}

// OldestOperations returns when the oldest write and read operation still open
// on the mounted block store storeID began, each zero for none, as
// block_store_writeback.MarkingStore.OldestOperations reports them. A store
// not mounted has none open.
func OldestOperations(providerID, accountID, storeID string) (write, read time.Time) {
	// Find the open mount.
	s := lookupSweep(adoptKey{volumeID: StorageVolumeID(providerID, accountID), storeID: storeID})
	s.mtx.Lock()
	wb := s.wb
	s.mtx.Unlock()
	if wb == nil {
		return time.Time{}, time.Time{}
	}

	// Report its oldest open operations.
	ops := wb.OldestOperations()
	return ops.Write, ops.Read
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

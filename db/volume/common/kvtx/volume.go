package kvtx

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/kvtx"
	hstore "github.com/s4wave/spacewave/db/store"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// StatsFn returns storage usage statistics for a volume.
type StatsFn func(ctx context.Context) (*volume.StorageStats, error)

// Volume implements a key-value volume.
type Volume struct {
	// volumeID is the volume id.
	volumeID string
	// Store is the hydra store.
	hstore.Store
	// Coordinator coordinates direct multi-writer ObjectStore access.
	coord.Coordinator
	// Peer indicates the volume has a peer identity.
	peer.Peer
	// kvtxStore is the underlying kvtx store.
	kvtxStore kvtx.Store
	// kvKey is the underlying kvkey.
	kvKey *store_kvkey.KVKey
	// procID names this process's reader pins and stages, unique per open.
	procID string
	// clock returns the time journal records, leases and the grace period are
	// read on, nil for time.Now.
	clock func() time.Time
	// statsFn returns storage stats, may be nil.
	statsFn StatsFn
	// statsBcast wakes watchers when storage stats may have changed.
	statsBcast broadcast.Broadcast
	// closeFn is the close func, may be nil.
	closeFn func() error
	// deleteFn removes the backing store after Close, may be nil.
	deleteFn func() error
	// collector frees what no held named root reaches, nil without atomic
	// commits.
	collector *block_gc.Collector
	// adoptMu runs one adoption of this Volume at a time.
	adoptMu sync.Mutex
	// atomic is set when the store commits transactions atomically, which
	// the Volume's liveness, roots, pins and stages need.
	atomic bool
	// ordered flushes direct atomic writes committed with write ordering, may
	// be nil.
	ordered kvtx.OrderedCommitStore
	// atomicHashGet records whether hash verification reads are enabled for
	// atomic publication block construction.
	atomicHashGet bool
	// closeBcast guards the closing, closed, and closeErr state below.
	closeBcast broadcast.Broadcast
	// closing records that Close has started and direct writes must stop.
	closing bool
	// closed records that the close sequence finished.
	closed bool
	// directMu joins synchronous preparation and GC transactions before closing.
	directMu sync.RWMutex
	// directClosed records that Close joined direct transactions, guarded by
	// directMu.
	directClosed bool
	// fills groups concurrent cache fills into shared transactions.
	fills fillQueue
	// closeErr stores the error from Close.
	closeErr error
	// rootPinMu guards reader pin counts and stages. Direct transactions take
	// it to carry released pins, so it is never held while opening a volume
	// transaction.
	rootPinMu csync.Mutex
	// rootPins counts the reader pins by root name.
	rootPins map[string]*rootPin
	// idlePins are the reader pins with no reader and no write in flight,
	// which the next changing direct transaction releases, by root name.
	idlePins map[string]*rootPin
	// stages are the open stages by name, and released stages whose roots no
	// commit has released yet.
	stages map[string]*openStage
	// releasedStages names the released stages whose roots no commit has
	// released yet.
	releasedStages map[string]struct{}
	// rootPinsClosed records that Close released the reader pins.
	rootPinsClosed bool
	// renewCancel stops the renewal of leased roots, nil without one.
	renewCancel context.CancelFunc
	// renewDone closes when the renewal has stopped.
	renewDone chan struct{}
	// legacyCancel stops the legacy graph sweep, nil without one.
	legacyCancel context.CancelFunc
	// legacyDone closes when the legacy graph sweep has stopped.
	legacyDone chan struct{}
	// adoptCancel stops the background adoption, nil without one.
	adoptCancel context.CancelFunc
	// adoptDone closes when the background adoption has stopped.
	adoptDone chan struct{}
	// adoptWake holds one pending request for a background adoption pass,
	// nil without a background adoption.
	adoptWake chan struct{}
	// proofMu guards the pending root proofs and the free count. It is never
	// held across a volume transaction.
	proofMu sync.Mutex
	// rootProofs are the completion proofs no commit has carried yet, keyed by
	// block key.
	rootProofs map[string]*block.BlockRef
	// frees counts the collector transactions that may have freed blocks.
	frees uint64
	// healthBcast guards health and wakes its watchers when it changes.
	healthBcast broadcast.Broadcast
	// health is the collection phase and the measured sizes of the volume.
	health healthState
	// healthSizes measures the storage's sizes, may be nil.
	healthSizes HealthSizesFn
	// healthWake asks the size measurement to run soon, after a collection
	// may have freed space.
	healthWake chan struct{}
	// healthCancel stops the size measurement, may be nil.
	healthCancel context.CancelFunc
	// healthDone closes when the size measurement has stopped.
	healthDone chan struct{}
	// reserved mirrors health.reserve for the new-data entry points, which
	// read it without healthBcast.
	reserved atomic.Bool
	// writtenBytes counts the new data the volume admitted or committed, for
	// the size measurement.
	writtenBytes atomic.Uint64
	// healthDueAt is the writtenBytes at which the next size measurement is
	// due, zero before the first.
	healthDueAt atomic.Uint64
	// puts counts the new blocks of each write path, for the flows.
	puts [numPutPaths]putCount
	// le logs the first caller of each write path putting a block without
	// its references, may be nil.
	le *logrus.Entry
	// storageFlows fills in the flows the storage counts, may be nil.
	storageFlows func(*volume.VolumeFlows)
}

// KvtxVolume is an interface for a volume with a kvtx store.
type KvtxVolume interface {
	// KvtxVolume extends Volume
	volume.Volume

	// GetKvtxStore returns the underlying kvtx store.
	GetKvtxStore() kvtx.Store
	// GetKvKey returns the instance of KvKey used to build keys.
	GetKvKey() *store_kvkey.KVKey
}

// NewVolume builds a new key-value volume.
//
// store /may/ optionally also be a store_kvtx.Store.
func NewVolume(
	ctx context.Context,
	storeID string,
	kvkey *store_kvkey.KVKey,
	store kvtx.Store,
	conf *store_kvtx.Config,
	noGenerateKey,
	noWriteKey bool,
	statsFn StatsFn,
	closeFn func() error,
	deleteFn ...func() error,
) (*Volume, error) {
	// Build the Volume around its key-value store and cleanup callbacks. A
	// BlockDomain serves the blocks outside a transaction.
	v := &Volume{
		Store:     store_kvtx.NewKVTx(kvkey, store, conf),
		kvtxStore: store,
		kvKey:     kvkey,
		statsFn:   statsFn,
		closeFn:   closeFn,
		health:    healthState{phase: volume.VolumeGCPhase_VOLUME_GC_PHASE_IDLE, phaseStart: time.Now()},
	}
	if domain, ok := store.(BlockDomain); ok {
		v.Store = store_kvtx.NewKVTxWithBlockStore(kvkey, store, domain.GetBlockStore(), conf)
	}
	if len(deleteFn) != 0 {
		v.deleteFn = deleteFn[0]
	}

	// Load or claim the Volume identity and initialize its storage.
	v, err := initVolume(ctx, v, storeID, noGenerateKey, noWriteKey)
	if err != nil {
		return nil, err
	}

	// Coordinate direct writers using the initialized Volume identity.
	v.Coordinator = coord_inmem.ForVolume(v.GetID())

	// Keep liveness, roots, pins and stages on a store that commits atomically.
	if atomicStore, ok := store.(kvtx.AtomicCommitStore); ok && atomicStore.SupportsAtomicCommit() {
		v.atomicHashGet = !conf.GetDisableHashGet()
		v.atomic = true
		if orderedStore, ok := store.(kvtx.OrderedCommitStore); ok {
			v.ordered = orderedStore
		}
		grace, err := parseGrace(conf.GetCollectorGraceDur())
		if err != nil {
			return nil, volume.Permanent(err)
		}
		v.collector = v.newCollector(conf.GetCollectorFrees(), grace)
		if err := v.startLiveness(ctx); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// newCollector returns the collector of the Volume, which frees when frees is
// set, with grace as its grace period, zero for the default.
func (v *Volume) newCollector(frees bool, grace time.Duration) *block_gc.Collector {
	opts := block_gc.CollectorOptions{
		Frees: frees,
		Grace: grace,
		Now:   v.now,
		Phase: v.collectorPhase,
	}
	opts.ForegroundWaiting = func() bool { return kvtx.ForegroundWaiting(v.kvtxStore) }
	return block_gc.NewCollector(collectorStore{v: v}, opts)
}

// parseGrace parses the collector's grace period, zero for the default. It
// refuses a grace no longer than the lease of a leased root, rootLease, as the
// Config field says, outside a test binary, where a test sets a shorter one
// to collect at once.
func parseGrace(dur string) (time.Duration, error) {
	// An empty setting uses the default.
	if dur == "" {
		return 0, nil
	}

	// Parse a positive duration.
	grace, err := time.ParseDuration(dur)
	if err != nil {
		return 0, errors.Join(errors.New("parse collector_grace_dur"), err)
	}
	if grace <= 0 {
		return 0, errors.New("collector_grace_dur must be positive")
	}

	// Require a grace past the root lease outside tests.
	if grace <= rootLease && !testing.Testing() {
		return 0, errors.New("collector_grace_dur " + grace.String() + " must exceed the root lease of " + rootLease.String())
	}
	return grace, nil
}

// startLiveness starts the renewal of this process's leased roots, returns a
// Volume an adoption left adopting to unadopted and, when the format 7
// upgrade left an old graph's records, starts their background sweep.
func (v *Volume) startLiveness(ctx context.Context) error {
	// Read the liveness state, and return a Volume an adoption that died with
	// its process left adopting to unadopted.
	var state *block_gc.LivenessState
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		var err error
		state, err = rec.GetState(ctx)
		return err
	})
	if err != nil {
		return err
	}
	if state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTING {
		if err := block_gc.ResetAdoption(ctx, collectorStore{v: v}); err != nil {
			return err
		}
	}

	// Adopt an unadopted Volume in the background when its collector frees,
	// as CollectorFrees describes: frees need exact counts, and without them
	// nothing reads the counts. Child lists an older format left unverified
	// adopt as they are, and the collector frees only blocks born since
	// until they are verified. The adoption also runs again whenever a list
	// rebuild returns the Volume to unadopted.
	if v.collector.Frees() {
		adoptCtx, cancelAdopt := context.WithCancel(context.WithoutCancel(ctx))
		v.adoptCancel, v.adoptDone = cancelAdopt, make(chan struct{})
		v.adoptWake = make(chan struct{}, 1)
		if state.GetAdoption() != block_gc.Adoption_ADOPTION_ADOPTED {
			v.wakeAdopt()
		}
		go v.runBackgroundAdopt(adoptCtx)
	}

	// Sweep an old graph's records when they remain.
	if state.GetLegacyGraph() {
		sweepCtx, cancelSweep := context.WithCancel(context.WithoutCancel(ctx))
		v.legacyCancel, v.legacyDone = cancelSweep, make(chan struct{})
		go v.runLegacySweep(sweepCtx)
	}

	// Renew the leased roots this process will hold, last, so a failed open
	// leaves no renewal running.
	renewCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	v.renewCancel, v.renewDone = cancel, make(chan struct{})
	go v.runRootRenewal(renewCtx)
	return nil
}

// initVolume brings the store to the current format and loads or claims the
// Volume identity.
func initVolume(
	ctx context.Context,
	v *Volume,
	storeID string,
	noGenerateKey,
	noWriteKey bool,
) (*Volume, error) {
	// Bring the store to the current format before any read.
	if _, err := upgradeFormat(ctx, v.kvtxStore, v.kvKey); err != nil {
		return nil, err
	}

	// Load the stored identity.
	peerPriv, err := v.LoadPeerPriv(ctx)
	if err != nil {
		return nil, err
	}

	// Generate an identity when none is stored, and store it unless another
	// mount of the same store committed one first.
	if peerPriv == nil {
		if noGenerateKey {
			return nil, errors.New("peer private key doesn't exist")
		}
		peerPriv, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}
		if !noWriteKey {
			peerPriv, err = v.claimPeerPriv(ctx, peerPriv)
			if err != nil {
				return nil, err
			}
		}
	}

	// Build the peer and derive the volume id from its peer id.
	v.Peer, err = peer.NewPeer(peerPriv)
	if err != nil {
		return nil, err
	}
	v.volumeID = volume.NewVolumeID(storeID, v.Peer.GetPeerID())

	// Name this process's pins and stages.
	v.procID = ulid.NewULID()
	v.rootPins = make(map[string]*rootPin)
	v.idlePins = make(map[string]*rootPin)
	v.stages = make(map[string]*openStage)
	v.releasedStages = make(map[string]struct{})
	return v, nil
}

// claimPeerPriv stores priv as the volume identity unless one is already
// stored, and returns the stored identity. The check and the write share one
// write transaction, so concurrent mounts of one store agree on the identity
// even when their earlier reads saw an older revision.
func (v *Volume) claimPeerPriv(ctx context.Context, priv crypto.PrivKey) (crypto.PrivKey, error) {
	// Read the identity under the writer.
	tx, err := v.kvtxStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	key := v.kvKey.GetPeerPrivKey()
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if found && len(data) != 0 {
		return keypem.ParsePrivKeyPem(data)
	}

	// Store the new identity.
	data, err = keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, err
	}
	if err := tx.Set(ctx, key, data); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return priv, nil
}

// GetID returns the computed volume id.
func (v *Volume) GetID() string {
	return v.volumeID
}

// GetPeerID returns the volume peer ID.
func (v *Volume) GetPeerID() peer.ID {
	return v.Peer.GetPeerID()
}

// GetPeer returns the Peer object.
// If withPriv=false ensure that the Peer returned does not have the private key.
func (v *Volume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	vp := v.Peer
	if !withPriv {
		return peer.NewPeerWithPubKey(vp.GetPubKey())
	}
	return vp, nil
}

// GetKvtxStore returns the underlying kvtx store.
func (v *Volume) GetKvtxStore() kvtx.Store {
	return v.kvtxStore
}

// GetKvKey returns the instance of KvKey used to build keys.
func (v *Volume) GetKvKey() *store_kvkey.KVKey {
	return v.kvKey
}

// GetStorageStats returns storage usage statistics for the volume.
func (v *Volume) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	if v.statsFn != nil {
		return v.statsFn(ctx)
	}
	return &volume.StorageStats{}, nil
}

// GetStorageStatsSnapshotWithWait returns storage usage statistics and a wait
// channel that closes when storage stats may have changed.
func (v *Volume) GetStorageStatsSnapshotWithWait(
	ctx context.Context,
) (*volume.StorageStats, <-chan struct{}, error) {
	// Subscribe to Volume statistics changes before reading the snapshot.
	var waitCh <-chan struct{}
	v.statsBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
	})

	// Read the Volume statistics paired with the change notification.
	stats, err := v.GetStorageStats(ctx)
	if err != nil {
		return nil, nil, err
	}
	return stats, waitCh, nil
}

// reader reads the embedded store, marking durable reads.
func (v *Volume) reader() storeReader {
	return storeReader{StoreOps: v.Store, volume: v}
}

// broadcastStorageStatsChanged wakes watchers of the Volume storage statistics.
func (v *Volume) broadcastStorageStatsChanged() {
	v.statsBcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		broadcastFn()
	})
}

// PutBlockBatch stores the entries in one transaction with their child lists,
// as block_gc.Land describes. A batch whose context carries a
// kvtx.BlockMarkRequest is stored with its markers in the same transaction. It
// returns volume.ErrReserve while the volume is in the reserve state, and
// block.ErrRefsUnknown for a block written without its child list.
func (v *Volume) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Store a batch that asked for marking with its markers in one transaction.
	if kvtx.GetBlockMarkRequest(ctx) != nil {
		return v.PrepareOwnedBlockBatch(ctx, "", entries)
	}

	// Refuse new data below the disk reserve, and blocks without their child
	// lists. A batch of tombstones only frees space, so it runs below the
	// reserve too.
	if !onlyTombstones(entries) {
		if err := v.checkReserve(ctx, v.gateBytes(batchBytes(entries))); err != nil {
			return nil, err
		}
	}
	v.noteUnknownRefsBatch(putDirect, entries)
	if err := requireRefsKnown(entries); err != nil {
		v.countRefused(putDirect, err)
		return nil, err
	}

	// Store the batch with its liveness, or as bytes alone on a store that
	// keeps none.
	var existed []bool
	var err error
	if v.HoldsRoots() {
		existed, err = v.putEntries(ctx, true, entries)
	} else {
		existed, err = v.reader().PutBlockBatch(ctx, entries)
	}
	if err != nil {
		return nil, err
	}
	v.countPutBatch(putDirect, entries, existed)
	if len(entries) != 0 {
		v.broadcastStorageStatsChanged()
	}
	return existed, nil
}

// PutBlock stores the block as PutBlockBatch does. It returns
// volume.ErrReserve while the volume is in the reserve state, and
// block.ErrRefsUnknown for a block written without its child list.
func (v *Volume) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Store a block that asked for marking with its marker in one transaction.
	putOpts, syncRequested := block.PutOptsWithoutSync(opts)
	if kvtx.GetBlockMarkRequest(ctx) != nil {
		ref, existed, err := v.PrepareOwnedBlock(ctx, "", data, putOpts)
		if err == nil && syncRequested {
			_, err = v.Sync(ctx)
		}
		return ref, existed, err
	}

	// Refuse new data below the disk reserve, and a block without its child
	// list.
	if err := v.checkReserve(ctx, v.gateBytes(len(data))); err != nil {
		return nil, false, err
	}
	v.noteUnknownRefs(putDirect, data, putOpts)
	if err := requireRefKnown(data, putOpts); err != nil {
		v.countRefused(putDirect, err)
		return nil, false, err
	}

	// Store the block with its liveness, or as bytes alone on a store that
	// keeps none.
	var ref *block.BlockRef
	var exists bool
	if v.HoldsRoots() {
		entry, err := putEntry(data, putOpts, v.GetHashType())
		if err != nil {
			return nil, false, err
		}
		existed, err := v.putEntries(ctx, true, []*block.PutBatchEntry{entry})
		if err != nil {
			return nil, false, err
		}
		ref, exists = entry.Ref, existed[0]
	} else {
		var err error
		ref, exists, err = v.Store.PutBlock(ctx, data, putOpts)
		if err != nil {
			return nil, exists, err
		}
	}

	// Count the block, and wake Volume statistics watchers when it adds
	// stored data.
	if ref != nil {
		v.countPut(putDirect, len(data), exists)
		if !exists {
			v.broadcastStorageStatsChanged()
		}
	}

	// Make the Volume durable when the block write requests synchronization.
	if syncRequested {
		if _, err := v.Sync(ctx); err != nil {
			return ref, exists, err
		}
	}
	return ref, exists, nil
}

// BeginReadOperation opens a read scope on the embedded store.
func (v *Volume) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	return v.reader().BeginReadOperation(ctx)
}

// GetBlock reads a block from the embedded store.
func (v *Volume) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return v.reader().GetBlock(ctx, ref)
}

// GetStoredBlock reads a block with its refs from the embedded store.
func (v *Volume) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return v.reader().GetStoredBlock(ctx, ref)
}

// RmBlock evicts a block's bytes and its proof, and reports the removal to the
// decoded-block cache. On a Volume that keeps liveness it keeps the block's
// child list and count, so evicting a live block leaves a hole the collector
// steps over and only the collector frees. Evict only bytes the store can
// fetch again, such as a writeback cache's blocks its remote holds: the
// proofs of the roots above the block stand.
func (v *Volume) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	// Evict through a tombstone, or remove from a store without liveness.
	var err error
	if v.HoldsRoots() {
		_, err = v.putEntries(ctx, true, []*block.PutBatchEntry{{Ref: ref, Tombstone: true}})
	} else {
		err = v.reader().RmBlock(ctx, ref)
	}
	if err != nil {
		return err
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// Sync forwards the durability barrier to the embedded store.
func (v *Volume) Sync(ctx context.Context) (bool, error) {
	// Synchronize block bytes through the embedded Volume store.
	fenced, err := v.Store.Sync(ctx)
	if err != nil {
		return false, err
	}

	// Flush ordered direct commits before waking statistics watchers.
	if v.ordered != nil {
		if err := v.ordered.Sync(ctx); err != nil {
			return false, err
		}
	}
	v.broadcastStorageStatsChanged()
	return fenced, nil
}

// OrdersWrites reports whether direct atomic writes commit with write
// ordering. Block and object store writes share one store, whose next full
// Commit makes every earlier ordered commit durable.
func (v *Volume) OrdersWrites() bool {
	return v.ordered != nil
}

// WaitDurable waits until every ordered commit completed before the call is
// durable, without forcing a flush.
func (v *Volume) WaitDurable(ctx context.Context) error {
	if v.ordered == nil {
		return nil
	}
	return v.ordered.WaitDurable(ctx)
}

// Maintenance lets a BlockDomain reclaim the space of removed blocks. Any
// other store reclaims it as it deletes their records.
func (v *Volume) Maintenance(ctx context.Context) error {
	// Report the maintenance, then measure the space it reclaimed.
	defer v.enterGCPhase(volume.VolumeGCPhase_VOLUME_GC_PHASE_MAINTENANCE)()
	defer v.wakeHealthSizes()
	if domain, ok := v.kvtxStore.(BlockDomain); ok {
		return domain.Maintenance(ctx)
	}
	return nil
}

// Close closes the volume, returning any errors.
// Close is idempotent: subsequent calls return the same error.
func (v *Volume) Close() error {
	// Claim the Volume close sequence or observe the existing closer.
	var waitCh <-chan struct{}
	started := false
	v.closeBcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Leave an already closed Volume unchanged.
		if v.closed {
			return
		}

		// Subscribe to completion when another caller is closing the Volume.
		if v.closing {
			waitCh = getWaitCh()
			return
		}

		// Stop new Volume writes and claim responsibility for cleanup.
		v.closing = true
		started = true
		broadcast()
	})

	// Join the existing Volume close sequence when another caller claimed it.
	if !started {
		if waitCh != nil {
			<-waitCh
		}
		return v.closeErr
	}

	// Stop the background liveness work, release root pins and join direct
	// Volume transactions before closing storage.
	if v.legacyCancel != nil {
		v.legacyCancel()
		<-v.legacyDone
	}
	if v.adoptCancel != nil {
		v.adoptCancel()
		<-v.adoptDone
	}
	if v.renewCancel != nil {
		v.renewCancel()
		<-v.renewDone
	}
	closeErr := v.closeRootPins()
	v.directMu.Lock()
	v.directClosed = true
	v.directMu.Unlock()

	// Stop measuring sizes, then close the reference graph and backing store.
	if v.healthCancel != nil {
		v.healthCancel()
		<-v.healthDone
	}
	if v.closeFn != nil {
		closeErr = errors.Join(closeErr, v.closeFn())
	}

	// Publish the Volume close result to all waiting callers.
	v.closeBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		v.closeErr = closeErr
		v.closed = true
		broadcast()
	})
	return v.closeErr
}

// Delete closes the volume and removes the backing store, then reports the
// removal of every block to the decoded-block cache.
func (v *Volume) Delete() error {
	if err := v.Close(); err != nil {
		return err
	}
	if v.deleteFn != nil {
		defer block.InvalidateAllRemovedBlocks()
		if err := v.deleteFn(); err != nil {
			return err
		}
	}
	v.broadcastStorageStatsChanged()
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume = (*Volume)(nil)
	_ KvtxVolume    = (*Volume)(nil)
)

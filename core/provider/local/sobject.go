package provider_local

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ErrStateChanged is returned by ReplaceSharedObjectState when the object's
// listed generation or stored state is not the one the caller read: someone
// wrote, deleted or created it again since. A mounted state write returns it
// when the stored state is not the one the mount holds: a replace committed
// since.
var ErrStateChanged = errors.New("shared object state changed")

// SharedObject implements the sobject interface attached to sobjectTracker.
type SharedObject struct {
	// ctx is the retained mount lifecycle.
	ctx context.Context
	// tkr retains the provider account and shared object state.
	tkr *sobjectTracker
	// run is the mount's write access to the tracker's health.
	run *sobjectRun

	// blkStore stores the shared object blocks.
	blkStore bstore.BlockStore
	// soHost owns accepted shared object state.
	soHost *sobject.SOHost
	// lsoHost runs local persistence and operations.
	lsoHost *LocalSOHost
	// objStore stores local shared object records.
	objStore object.ObjectStore
	// objStoreID names objStore in the account volume.
	objStoreID string
	// localPriv authenticates the local participant.
	localPriv crypto.PrivKey
	// localPid identifies the local participant.
	localPid peer.ID

	// joinRequestsMtx serializes join request writes.
	joinRequestsMtx csync.Mutex
	// joinRequests contains the persisted pending join requests.
	joinRequests *ccontainer.CContainer[*sobject.SOJoinRequestList]
	// body drives the attached body for the mount's lifetime.
	body *sobject.BodyHost
}

// GetSOHostState returns a snapshot of the current SOState via the SOHost.
func (s *SharedObject) GetSOHostState(ctx context.Context) (*sobject.SOState, error) {
	return s.soHost.GetHostState(ctx)
}

// GetBus returns the bus used for the shared object.
func (s *SharedObject) GetBus() bus.Bus {
	return s.tkr.a.t.p.b
}

// GetPeerID returns the local peer id for the shared object.
func (s *SharedObject) GetPeerID() peer.ID {
	return s.localPid
}

// GetSharedObjectID returns the shared object id.
func (s *SharedObject) GetSharedObjectID() string {
	return s.tkr.id
}

// GetBlockStore returns the block store mounted along with the SharedObject.
func (s *SharedObject) GetBlockStore() bstore.BlockStore {
	return s.blkStore
}

// GetBackingVolume returns the volume that owns this SharedObject's storage.
func (s *SharedObject) GetBackingVolume() volume.Volume {
	return s.tkr.a.vol
}

// WaitDurable waits until every state write completed before the call is
// durable, without forcing a flush.
func (s *SharedObject) WaitDurable(ctx context.Context) error {
	return s.soHost.WaitDurable(ctx)
}

// AccessLocalStateStore isolates state by SharedObject and store ID within the
// account's object store. Releasing the mount invalidates its local stores.
func (s *SharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	// Prefix the account object store and release the caller's callback with the mount.
	prefixedObjStore := object.NewPrefixer(s.objStore, s.localStatePrefix(storeID))
	if released == nil {
		return prefixedObjStore, func() {}, nil
	}
	relReleased := context.AfterFunc(s.ctx, released)
	return prefixedObjStore, func() { relReleased() }, nil
}

// mountStore is the account object store as one mount sees it. A handle
// writes keys under its object, such as its local state stores and join
// requests, only while its mount runs and the object has stored state, so a
// handle that outlives a delete writes nothing under the deleted prefix or
// under an object relisted with the same id.
type mountStore struct {
	object.ObjectStore
	// ctx is the mount's lifetime.
	ctx context.Context
	// hostKey is the object's stored state key.
	hostKey []byte
}

// NewTransaction opens a transaction. A write fails with context.Canceled once
// the mount has ended, and with ErrSharedObjectNotFound when the object has no
// stored state. A write transaction holds the store's writer from open to
// end, so no delete commits between the check and the write's commit.
func (m mountStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Refuse a handle whose mount has ended, so it neither reads nor writes
	// the keys of an object a later create lists under the same id.
	if err := m.ctx.Err(); err != nil {
		return nil, err
	}
	if !write {
		return m.ObjectStore.NewTransaction(ctx, false)
	}

	// Refuse a write to an object deleted since the mount began.
	tx, err := m.ObjectStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	found, err := tx.Exists(ctx, m.hostKey)
	if err == nil && !found {
		err = sobject.ErrSharedObjectNotFound
	}
	if err != nil {
		tx.Discard()
		return nil, err
	}
	return tx, nil
}

// localStatePrefix returns the object store prefix of the local state store
// storeID.
func (s *SharedObject) localStatePrefix(storeID string) []byte {
	return []byte("so/" + s.tkr.id + "/ls/" + storeID + "/")
}

// GetSharedObjectState returns a snapshot of the shared object state.
func (s *SharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	// Retain the local state watch until it supplies a usable snapshot.
	stateCtr, relStateCtr, err := s.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relStateCtr()

	// Wait no longer than the mount lives: an ended host publishes no value.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(s.ctx, cancel)()
	val, err := stateCtr.WaitValue(waitCtx, nil)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// AccessSharedObjectState adds a reference to the state and returns the state container.
// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
// An ended mount, such as one a delete restarted, serves no state and
// returns context.Canceled, as its writes do.
func (s *SharedObject) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	if err := s.ctx.Err(); err != nil {
		return nil, nil, err
	}
	return s.lsoHost.AccessSharedObjectState(ctx, released)
}

// AccessSharedObjectHealth retains the provider's health watch for this mount.
func (s *SharedObject) AccessSharedObjectHealth(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	ref, err := s.tkr.ref.Await(ctx)
	if err != nil {
		return nil, nil, err
	}
	return s.tkr.a.AccessSharedObjectHealth(ctx, ref, released)
}

// showReplay shows in the health what the body driver reports of this
// device's replay: its revoked edits, a checkpoint whose state differs from
// the replayed one, and the edits the sequencer has not placed.
func (s *SharedObject) showReplay(r *sobject.BodyReport) {
	s.run.swapHealth(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return health.WithRejectedEdits(r.RejectedEdits).WithCheckpointMismatch(r.CheckpointMismatch).WithUnorderedCount(r.Unordered)
	})
}

// SetBody waits for the previous body's release, then attaches the body the
// SharedObject agrees on until release. The mount drives the body through its
// replay, signs the checkpoints this device owes and votes in the group's
// decisions while the body is attached.
func (s *SharedObject) SetBody(ctx context.Context, body sobject.Body) (func(), error) {
	return s.body.SetBody(ctx, body)
}

// QueueOperation signs op as the local participant and adds it to the
// operation set. Returns the local operation ID.
func (s *SharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	return s.lsoHost.QueueOperation(ctx, op)
}

// sobjectTracker tracks a SharedObject in the ProviderAccount.
type sobjectTracker struct {
	// a is the provider account.
	a *ProviderAccount
	// id is the shared object ID.
	id string
	// ref is the shared object reference, set when instantiating the tracker.
	ref *promise.Promise[*sobject.SharedObjectRef]
	// res holds the mount result and health the runs publish.
	res *sobjectResult
	// awaitingState is set from before the routine reads the stored state
	// until the read finds it, and stays set while the routine refuses the
	// mount for lack of state; reviveRefusedSharedObject restarts it while set.
	awaitingState atomic.Bool
	// peerStateBcast guards peerStates.
	peerStateBcast broadcast.Broadcast
	// peerStates maps each sync peer to the digest of the last state it
	// advertised as held. Entries are peer claims, used only to delay work.
	peerStates map[string][]byte
}

// recordPeerState records the digest of a state a sync peer advertised.
func (t *sobjectTracker) recordPeerState(remoteID peer.ID, stateHash []byte) {
	t.peerStateBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.peerStates == nil {
			t.peerStates = make(map[string][]byte)
		}
		t.peerStates[remoteID.String()] = stateHash
		broadcast()
	})
}

// buildSharedObjectTracker builds a new sobjectTracker for a sobject id.
func (a *ProviderAccount) buildSharedObjectTracker(sobjectID string) (keyed.Routine, *sobjectTracker) {
	tracker := &sobjectTracker{
		a:   a,
		id:  sobjectID,
		ref: promise.NewPromise[*sobject.SharedObjectRef](),
		res: newSobjectResult(),
	}
	return tracker.executeSharedObjectTracker, tracker
}

// sobjectResult holds a tracker's mount result and health. Readers await and
// watch it; only runs write it, each through the sobjectRun it takes at
// start. Taking a run, and retire, move the generation on, which retires
// every earlier run, so a write can come only from the newest run.
//
// A restart chain can leave two runs of one object alive at once: keyed
// starts a run once its predecessor exits, but a predecessor cancelled while
// waiting exits at once, so the next run waits only on it, while the run
// before that may still be tearing down. That older run is harmless. Its
// handle is retired, so it publishes, clears and changes health nothing, and
// its context is cancelled, so its state and store writes refuse: the state
// lock and mountStore both check it. What it still does is release its own
// block store and object store references.
type sobjectResult struct {
	// mtx orders the generation with every write below.
	mtx sync.Mutex
	// gen is the generation of the only run that may write.
	gen uint64
	// prom holds the newest run's mount result.
	prom *promise.PromiseContainer[*SharedObject]
	// so is the SharedObject prom holds, if any; reset ends its host.
	so *SharedObject
	// health holds the newest run's health snapshot.
	health *ccontainer.CContainer[*sobject.SharedObjectHealth]
}

// sobjectRun is one run's write access to its tracker's sobjectResult. Every
// method of a run a later one or a retire has replaced does nothing.
type sobjectRun struct {
	// res is the result the run writes.
	res *sobjectResult
	// gen is the generation the run took at start.
	gen uint64
	// ctx is the run's context; a run publishes only while it is live.
	ctx context.Context
}

// newSobjectResult builds a result with no mount result, loading.
func newSobjectResult() *sobjectResult {
	return &sobjectResult{
		prom: promise.NewPromiseContainer[*SharedObject](),
		health: ccontainer.NewCContainer(sobject.NewSharedObjectLoadingHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		)),
	}
}

// await waits for the newest run's mount result.
func (r *sobjectResult) await(ctx context.Context) (*SharedObject, error) {
	return r.prom.Await(ctx)
}

// watchHealth returns the health snapshot for reading and watching.
func (r *sobjectResult) watchHealth() ccontainer.Watchable[*sobject.SharedObjectHealth] {
	return r.health
}

// start retires every earlier run, clears the mount result, reports loading
// and returns the new run's handle, bound to the run's context.
func (r *sobjectResult) start(ctx context.Context) *sobjectRun {
	// Move the generation on and reset under the lock, retiring earlier runs.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.gen++
	r.reset()
	return &sobjectRun{res: r, gen: r.gen, ctx: ctx}
}

// retire retires the current run, clears the mount result and reports
// loading, so a waiter waits for the next run and a health watcher does not
// see the retired run's health. restartSharedObject calls it before each
// restart.
func (r *sobjectResult) retire() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.gen++
	r.reset()
}

// reset ends the published SharedObject's state snapshot, clears the mount
// result and reports loading. The caller holds mtx.
func (r *sobjectResult) reset() {
	if r.so != nil {
		r.so.lsoHost.end()
		r.so = nil
	}
	r.prom.SetPromise(nil)
	r.health.SetValue(sobject.NewSharedObjectLoadingHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	))
}

// current reports whether the run may write. The caller holds res.mtx.
func (h *sobjectRun) current() bool {
	return h.res.gen == h.gen
}

// publish sets the run's health and mount result while the run is current
// and its context live.
func (h *sobjectRun) publish(health *sobject.SharedObjectHealth, so *SharedObject, err error) {
	// Write only as the newest run, while its context is live.
	h.res.mtx.Lock()
	defer h.res.mtx.Unlock()
	if !h.current() || h.ctx.Err() != nil {
		return
	}
	h.res.health.SetValue(health)
	h.res.so = so
	h.res.prom.SetResult(so, err)
}

// swapHealth applies fn to the health while the run is current and its
// context live.
func (h *sobjectRun) swapHealth(fn func(*sobject.SharedObjectHealth) *sobject.SharedObjectHealth) {
	h.res.mtx.Lock()
	defer h.res.mtx.Unlock()
	if !h.current() || h.ctx.Err() != nil {
		return
	}
	h.res.health.SwapValue(fn)
}

// end clears the run's published mount result as it exits, while the run is
// current; its context has usually ended by then.
func (h *sobjectRun) end() {
	h.res.mtx.Lock()
	defer h.res.mtx.Unlock()
	if h.current() {
		h.res.prom.SetPromise(nil)
	}
}

// executeSharedObjectTracker executes the sobjectTracker for the sobject.
func (t *sobjectTracker) executeSharedObjectTracker(rctx context.Context) (rerr error) {
	// Take this run's handle, which retires every earlier run, and report a
	// terminal mount error through it.
	if testHookRunStarting != nil {
		testHookRunStarting(t.id)
	}
	run := t.res.start(rctx)
	defer func() {
		if rerr != nil && rerr != context.Canceled {
			run.publish(sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				rerr,
			), nil, rerr)
		}
	}()

	// Tie every retained store and state component to this mount's context.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	le := t.a.le.WithField("sobject-id", t.id)
	le.Debug("mounting sobject")

	// Resolve the provider and storage identities from the mounted reference.
	sobjectRef, err := t.ref.Await(ctx)
	if err != nil {
		return err
	}
	provRef := sobjectRef.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sharedObjectID := provRef.GetId()

	// Retain the SharedObject's block store for the mount lifetime.
	blkStore, blkStoreRef, err := bstore.ExMountBlockStore(
		ctx,
		t.a.t.p.b,
		NewBlockStoreRef(
			providerID,
			providerAccountID,
			sobjectRef.GetBlockStoreId(),
		),
		false,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer blkStoreRef.Release()
	le.Debug("mounted block store for sobject successfully")

	// Create and/or open the object store in the account volume.
	volID := t.a.vol.GetID()
	objectStoreID := SobjectObjectStoreID(
		providerID,
		providerAccountID,
	)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		t.a.t.p.b,
		false,
		objectStoreID,
		volID,
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer diRef.Release()
	le.Debug("mounted object store for sobject successfully")

	// Get the peer id from the volume for ops.
	localPeer, err := t.a.vol.GetPeer(ctx, true)
	if err != nil {
		return err
	}
	localPeerID := localPeer.GetPeerID()

	// Obtain the local signing key for state and operation ownership.
	localPriv, err := localPeer.GetPrivKey(ctx)
	if err != nil {
		return err
	}

	// Load the stored state. An unlisted object with none was deleted or never
	// created, and an arriving one waits for its transfer: refuse the mount
	// until every waiter lets go, or the transfer's write restarts it.
	// awaitingState is set before the read, so a writer that stores state
	// after the read finds it set and restarts this routine.
	objStore := objStoreHandle.GetObjectStore()
	t.awaitingState.Store(true)
	if err := t.loadSharedObjectState(ctx, objStore, sharedObjectID); err != nil {
		if !errors.Is(err, sobject.ErrSharedObjectNotFound) && !errors.Is(err, sobject.ErrSharedObjectArriving) {
			return err
		}
		if testHookStateRefused != nil {
			testHookStateRefused(sharedObjectID)
		}
		run.publish(sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
			err,
		), nil, err)
		<-ctx.Done()
		return context.Canceled
	}
	t.awaitingState.Store(false)

	// Share the accepted-state watch and lock with the local persistence owner.
	// State writes share the account volume, which reports their durability.
	watchFn, lockFn, syncFuncs := NewObjectStoreSOStateFuncs(ctx, objStore, localPeerID)
	vol := t.a.vol
	syncFuncs.WaitDurable = func(ctx context.Context) error {
		return volume.WaitDurable(ctx, vol)
	}
	soHost := sobject.NewSOHost(ctx, watchFn, lockFn, sharedObjectID, syncFuncs)

	// Construct the local publisher and mounted SharedObject handle.
	lsoHost, err := NewLocalSOHost(
		ctx,
		le,
		localPriv,
		soHost,
		sharedObjectID,
		t.a.t.p.sfs,
	)
	if err != nil {
		return err
	}
	joinRequests, err := readJoinRequests(ctx, objStore, sharedObjectID)
	if err != nil {
		return err
	}
	so := &SharedObject{
		ctx:          ctx,
		tkr:          t,
		run:          run,
		blkStore:     blkStore,
		soHost:       soHost,
		lsoHost:      lsoHost,
		objStore:     mountStore{ObjectStore: objStore, ctx: ctx, hostKey: SobjectObjectStoreHostStateKey(sharedObjectID)},
		objStoreID:   objectStoreID,
		localPriv:    localPriv,
		localPid:     localPeerID,
		joinRequests: ccontainer.NewCContainer(joinRequests),
	}
	so.body = sobject.NewBodyHost(le, so, so.showReplay)
	so.body.SetSigner(soHost, localPriv, t.a.t.p.sfs)
	so.body.SetContext(ctx)

	// A mounted local SharedObject is ready only after LocalSOHost publishes its
	// first state snapshot; otherwise immediate callers can block on state reads.
	errCh := make(chan error, 1)
	go func() {
		errCh <- lsoHost.Execute(ctx)
	}()
	stateCtr, relStateCtr, err := lsoHost.AccessSharedObjectState(ctx, nil)
	if err != nil {
		ctxCancel()
		return err
	}
	defer relStateCtr()
	if _, err := stateCtr.WaitValue(ctx, errCh); err != nil {
		ctxCancel()
		return err
	}

	// Publish readiness only while the local persistence owner remains active.
	run.publish(sobject.NewSharedObjectReadyHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
	), so, nil)
	defer func() {
		if testHookRunEnding != nil {
			testHookRunEnding(t.id, run.end)
			return
		}
		run.end()
	}()
	return <-errCh
}

// createSharedObjectLocked creates a new sobject with the given details and a
// new generation, and returns its list entry. Each step runs in the
// transaction that lists the object, after its genesis state is written,
// with the new list entry. An id an enrollment is writing fails with errSOEnrolling, and one with keys
// under its prefix fails with errStaleSOKeys; see withClearedSOKeys. Assumes
// p.mtx is locked.
func (a *ProviderAccount) createSharedObjectLocked(ctx context.Context, id string, meta *sobject.SharedObjectMeta, extra ...func(context.Context, kvtx.Tx, *sobject.SharedObjectListEntry) error) (*sobject.SharedObjectListEntry, error) {
	// Derive the SharedObject and block-store references from account identity.
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()
	blockStoreID := SobjectBlockStoreID(id)
	sobjectRef := sobject.NewSharedObjectRef(providerID, providerAccountID, id, blockStoreID)
	if err := sobjectRef.Validate(); err != nil {
		return nil, err
	}

	// Reject invalid metadata before changing the account's stores.
	if err := meta.Validate(); err != nil {
		return nil, err
	}

	// Get the current object list.
	sharedObjectList := a.soListCtr.GetValue().CloneVT()
	if sharedObjectList == nil {
		sharedObjectList = &sobject.SharedObjectList{}
	}

	// Check the shared object id does not already exist.
	for _, soListEntry := range sharedObjectList.GetSharedObjects() {
		if soListEntry.GetRef().GetProviderResourceRef().GetId() == id {
			return nil, sobject.ErrSharedObjectExists
		}
	}

	// Leave an id an enrollment is writing to the enrollment.
	if a.enrolling[id] != 0 {
		return nil, errors.Wrapf(errSOEnrolling, "shared object %s", id)
	}

	// Create backing storage before publishing the SharedObject.
	if _, err := a.createBlockStoreLocked(ctx, blockStoreID); err != nil {
		return nil, err
	}

	// Append to the list of shared objects.
	entry := newListEntry(sobjectRef, meta, "", nil)
	insertListEntry(sharedObjectList, entry)

	// Finish an earlier lifetime's pending release, so the new object does
	// not adopt its roots.
	if err := a.finishPendingReleaseLocked(ctx, id); err != nil {
		return nil, err
	}

	// Persist the genesis state with the complete list, so a new object always
	// has state, before publishing the in-memory snapshot. An unlisted id
	// that no enrollment writes owns no key, so the write refuses any it
	// finds, which the caller clears first. The old behavior writes only the
	// list.
	var steps []func(context.Context, kvtx.Tx) error
	if !deleteStateOff {
		writeGenesis, err := a.genesisSOStateWriter(ctx, id)
		if err != nil {
			return nil, err
		}
		steps = append(steps, refuseStaleSOKeys(id), writeGenesis)
	}
	for _, step := range extra {
		steps = append(steps, func(ctx context.Context, tx kvtx.Tx) error {
			return step(ctx, tx, entry)
		})
	}
	if err := a.writeSharedObjectList(ctx, sharedObjectList, steps...); err != nil {
		return nil, err
	}
	a.soListCtr.SetValue(sharedObjectList)

	// Restart any live mount of the id after the commit, as a delete does,
	// so no handle keeps building on state the create replaced, such as an
	// enrollment's placeholder, and a mount that refused loads the genesis.
	a.restartSharedObject(id, nil)
	return entry, nil
}

// newListEntry returns the list entry that first lists an object in this
// account. generation is the account catalog's for an install of a
// cataloged lifetime; nil mints a new one. This is the only place a
// generation is minted: every later write and publish reads it from the list.
// See SharedObjectListEntry.Generation.
func newListEntry(ref *sobject.SharedObjectRef, meta *sobject.SharedObjectMeta, source string, generation *string) *sobject.SharedObjectListEntry {
	entry := &sobject.SharedObjectListEntry{
		Ref:    ref.CloneVT(),
		Source: source,
		Meta:   meta.CloneVT(),
	}
	if generation != nil {
		entry.Generation = *generation
	} else {
		entry.Generation = ulid.NewULID()
	}
	return entry
}

// insertListEntry adds a copy of entry to list in id order.
func insertListEntry(list *sobject.SharedObjectList, entry *sobject.SharedObjectListEntry) {
	list.SharedObjects = append(list.SharedObjects, entry.CloneVT())
	slices.SortFunc(list.SharedObjects, func(a, b *sobject.SharedObjectListEntry) int {
		return strings.Compare(a.GetRef().GetProviderResourceRef().GetId(), b.GetRef().GetProviderResourceRef().GetId())
	})
}

// CreateSharedObject creates a new sobject with the given details.
func (a *ProviderAccount) CreateSharedObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, _, _ string) (*sobject.SharedObjectRef, error) {
	return a.createSharedObject(ctx, id, meta)
}

// createSharedObject creates the sobject id, running each step in the
// transaction that lists it, and publishes it to the account catalog.
func (a *ProviderAccount) createSharedObject(ctx context.Context, id string, meta *sobject.SharedObjectMeta, steps ...func(context.Context, kvtx.Tx, *sobject.SharedObjectListEntry) error) (*sobject.SharedObjectRef, error) {
	// Clear the state an earlier lifetime abandoned, a bounded batch per hold
	// of the lock, then serialize local storage creation with the account's
	// other mutations.
	var entry *sobject.SharedObjectListEntry
	err := a.withClearedSOKeys(ctx, id, a.clearAbandonedSOKeys, func() error {
		// Create under the lock.
		relMtx, err := a.mtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer relMtx()
		entry, err = a.createSharedObjectLocked(ctx, id, meta, steps...)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Publish canonical account membership after the local mutation commits,
	// with the entry's new generation, which no tombstone has deleted.
	if err := a.publishAccountCatalogEntry(ctx, entry, false); err != nil {
		return nil, err
	}
	return entry.GetRef(), nil
}

// UpdateSharedObjectMeta updates the metadata for an existing shared object
// and publishes the edited entry, generation included, to the catalog.
func (a *ProviderAccount) UpdateSharedObjectMeta(ctx context.Context, id string, meta *sobject.SharedObjectMeta) error {
	// Edit the listed entry, then publish the edit.
	if err := meta.Validate(); err != nil {
		return err
	}
	var edited *sobject.SharedObjectListEntry
	err := a.editListedSharedObject(ctx, id, func(entry *sobject.SharedObjectListEntry) bool {
		entry.Meta = meta.CloneVT()
		edited = entry.CloneVT()
		return true
	})
	if err != nil {
		return err
	}
	return a.publishAccountCatalogEntry(ctx, edited, false)
}

// MountSharedObject retains a ready SharedObject until the returned release runs.
func (a *ProviderAccount) MountSharedObject(ctx context.Context, ref *sobject.SharedObjectRef, released func()) (sobject.SharedObject, func(), error) {
	// Retain the tracker for the validated SharedObject identity.
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}
	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)
	tkr.ref.SetResult(ref, nil)

	// Await mount readiness while preserving the caller's tracker reference.
	ws, err := tkr.res.await(ctx)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}
	if released == nil {
		return ws, tkrRef.Release, nil
	}

	// Report the handle invalid once its mount run ends, such as after a
	// delete restarts the tracker, so no caller keeps reading an ended mount.
	stop := context.AfterFunc(ws.ctx, released)
	return ws, func() {
		stop()
		tkrRef.Release()
	}, nil
}

// AccessSharedObjectHealth adds a reference to shared object health by ref.
func (a *ProviderAccount) AccessSharedObjectHealth(
	ctx context.Context,
	ref *sobject.SharedObjectRef,
	released func(),
) (ccontainer.Watchable[*sobject.SharedObjectHealth], func(), error) {
	// Validate the reference and retain the shared-object health watch.
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	// Track the shared object while the caller watches its health.
	sobjectID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sobjects.AddKeyRef(sobjectID)
	tkr.ref.SetResult(ref, nil)
	return tkr.res.watchHealth(), func() {
		tkrRef.Release()
		if released != nil {
			released()
		}
	}, nil
}

// AccessSharedObjectList adds a reference to the list of shared objects and returns the container.
// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
func (a *ProviderAccount) AccessSharedObjectList(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SharedObjectList], func(), error) {
	return a.soListCtr, func() {}, nil
}

// RefreshSharedObjectList keeps the SharedObjectProvider contract uniform.
// Local provider lists are updated synchronously by local mutations.
func (a *ProviderAccount) RefreshSharedObjectList(context.Context) error {
	return nil
}

// loadSharedObjectState validates the object's stored state. An object the
// account lists with no stored state, such as one listed before create stored
// a genesis, gets a genesis in a transaction that rereads the list. An
// unlisted object with none was deleted or never created and returns
// ErrSharedObjectNotFound. Under DeleteStateEnv's old behavior any object with
// no stored state gets a genesis.
func (t *sobjectTracker) loadSharedObjectState(ctx context.Context, objStore object.ObjectStore, sharedObjectID string) error {
	// Validate the stored state, or learn whether the object is listed.
	key := SobjectObjectStoreHostStateKey(sharedObjectID)
	var found, listed bool
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Refuse an object a transfer is still filling.
			if err := refuseArriving(ctx, tx, sharedObjectID); err != nil {
				return err
			}

			// Read the stored state; with none, read whether it is listed.
			data, ok, err := tx.Get(ctx, key)
			if err != nil {
				return err
			}
			if found = ok; found {
				val := &sobject.SOState{}
				if err := val.UnmarshalVT(data); err != nil {
					return err
				}
				return val.Validate(sharedObjectID)
			}
			listed, err = storedListsSharedObject(ctx, tx, sharedObjectID)
			return err
		},
	)
	if err != nil || found {
		return err
	}
	if !listed && !deleteStateOff {
		return sobject.ErrSharedObjectNotFound
	}

	// Store a genesis while the object is still listed, after clearing keys
	// an earlier lifetime left.
	if testHookStateListed != nil {
		testHookStateListed(sharedObjectID)
	}
	write, err := t.a.genesisSOStateWriter(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	return t.a.withClearedSOKeys(ctx, sharedObjectID, t.a.clearStaleSOKeys, func() error {
		return kvtx.RunTransaction(ctx, true,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, true)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				if !deleteStateOff {
					listed, err := storedListsSharedObject(ctx, tx, sharedObjectID)
					if err != nil {
						return err
					}
					if !listed {
						return sobject.ErrSharedObjectNotFound
					}
				}
				return write(ctx, tx)
			},
		)
	})
}

// writeGenesisSOState stores a genesis state for id unless state is already
// stored, in its own transaction under mtx. Enrollment uses it for the
// placeholder it mounts before it lists the object. Assumes mtx is not locked.
func (a *ProviderAccount) writeGenesisSOState(ctx context.Context, id string) error {
	// Build the genesis outside the lock.
	write, err := a.genesisSOStateWriter(ctx, id)
	if err != nil {
		return err
	}

	// After clearing keys an earlier lifetime left, under the lock, finish
	// its pending release, then store the genesis unless state is already
	// there, so a mount of the placeholder never adopts its roots.
	err = a.withClearedSOKeys(ctx, id, a.clearStaleSOKeys, func() error {
		// Release and write under the lock.
		relMtx, err := a.mtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer relMtx()
		if err := a.finishPendingReleaseLocked(ctx, id); err != nil {
			return err
		}
		return a.runSoObjectStoreTx(ctx, true, write)
	})
	if err != nil {
		return err
	}
	a.reviveRefusedSharedObject(id)
	return nil
}

// restartSharedObject restarts the tracker of id, if it runs and cond is nil
// or holds for it. Under the keyed lock, before the restart, it retires the
// running run and drops the published mount result, so a mount that follows
// the call waits for the new run rather than reading the old run's result,
// and the old run writes nothing more. A restart keyed makes on its own, a
// retry or a new context, needs no call here: the new run's start retires the
// old one.
func (a *ProviderAccount) restartSharedObject(id string, cond func(*sobjectTracker) bool) {
	a.sobjects.RestartRoutine(id, func(_ string, t *sobjectTracker) bool {
		if cond != nil && !cond(t) {
			return false
		}
		t.res.retire()
		return true
	})
}

// reviveRefusedSharedObject restarts the tracker of id if it refused a mount
// for lack of stored state, or is reading the state and might, so its waiters
// and watchers load the state just stored. A caller that stores state for id
// calls it after the commit. The tracker sets awaitingState before its read,
// so either the read sees the commit or this call sees the flag.
func (a *ProviderAccount) reviveRefusedSharedObject(id string) {
	a.restartSharedObject(id, func(t *sobjectTracker) bool {
		return t.awaitingState.Load()
	})
}

// genesisSOStateWriter builds a genesis state for id, owned by the account's
// storage peer and pinned to its signed genesis, and returns a transaction
// step that stores it with its config history unless state is already
// stored. Every path that gives an object its first state uses it. Keys under
// the prefix with no stored state belong to no object, so the step refuses
// them with errStaleSOKeys rather than deleting an unbounded prefix in the
// caller's transaction; the caller runs it through withClearedSOKeys.
// The genesis is built here, outside the step, so a retried transaction
// stores the same state rather than minting new key material.
func (a *ProviderAccount) genesisSOStateWriter(ctx context.Context, id string) (func(context.Context, kvtx.Tx) error, error) {
	// Resolve the storage peer's key.
	storagePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	priv, err := storagePeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}

	// Build the genesis state.
	le := a.le.WithField("sobject-id", id)
	le.Debug("initializing shared object with empty state")
	state, genesis, err := sobject.BuildGenesisSOState(le, a.t.p.sfs, id, priv, nil)
	if err != nil {
		return nil, err
	}
	data, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Store it with its history unless state is already stored.
	key := SobjectObjectStoreHostStateKey(id)
	return func(ctx context.Context, tx kvtx.Tx) error {
		// Keep state already stored; refuse keys an earlier lifetime left.
		found, err := tx.Exists(ctx, key)
		if err != nil || found {
			return err
		}
		if !deleteStateOff {
			if err := refuseStaleSOKeys(id)(ctx, tx); err != nil {
				return err
			}
		}
		if err := WriteSOConfigHistory(ctx, tx, id, genesis.GetConfig(), state.GetConfig(), []*sobject.SOConfigChange{genesis}); err != nil {
			return err
		}
		return tx.Set(ctx, key, data)
	}, nil
}

// buildSoObjectStore builds the shared object store for the provider account.
func (a *ProviderAccount) buildSoObjectStore(ctx context.Context) (object.ObjectStore, func(), error) {
	// Derive the account-wide object store from its provider identity.
	providerID := a.t.accountInfo.GetProviderId()
	providerAccountID := a.t.accountInfo.GetProviderAccountId()
	objectStoreID := SobjectObjectStoreID(providerID, providerAccountID)

	// Retain the mounted volume's object-store API for the caller.
	volID := a.vol.GetID()
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		objectStoreID,
		volID,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	return objStoreHandle.GetObjectStore(), diRef.Release, nil
}

// runSoObjectStoreTx runs fn in one transaction on the account object store,
// committing it when write is set.
func (a *ProviderAccount) runSoObjectStoreTx(ctx context.Context, write bool, fn func(context.Context, kvtx.Tx) error) error {
	store, release, err := a.buildSoObjectStore(ctx)
	if err != nil {
		return err
	}
	defer release()
	return kvtx.RunTransaction(ctx, write, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, write)
	}, fn)
}

// readSharedObjectList reads and returns the shared object list from storage.
func (a *ProviderAccount) readSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error) {
	var list *sobject.SharedObjectList
	err := a.runSoObjectStoreTx(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		list, err = readStoredSharedObjectList(ctx, tx)
		return err
	})
	return list, err
}

// readStoredSharedObjectList decodes the stored shared-object list in tx, or
// an empty list when absent.
func readStoredSharedObjectList(ctx context.Context, tx kvtx.Tx) (*sobject.SharedObjectList, error) {
	// Decode the stored list, or start from an empty one.
	data, found, err := tx.Get(ctx, SobjectObjectStoreListKey())
	if err != nil {
		return nil, err
	}
	list := &sobject.SharedObjectList{}
	if found {
		if err := list.UnmarshalVT(data); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// storedListsSharedObject reports whether the list stored in tx names id.
func storedListsSharedObject(ctx context.Context, tx kvtx.Tx, id string) (bool, error) {
	list, err := readStoredSharedObjectList(ctx, tx)
	if err != nil {
		return false, err
	}
	return listsSharedObject(list, id), nil
}

// writeSharedObjectList writes the shared object list to storage, running
// each step in the same transaction.
func (a *ProviderAccount) writeSharedObjectList(ctx context.Context, list *sobject.SharedObjectList, steps ...func(context.Context, kvtx.Tx) error) error {
	// Encode once and retry only the transactional write.
	data, err := list.MarshalVT()
	if err != nil {
		return err
	}
	return a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		for _, step := range steps {
			if err := step(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Set(ctx, SobjectObjectStoreListKey(), data)
	})
}

// GetSOHost returns the SOHost for invite operations.
func (s *SharedObject) GetSOHost() *sobject.SOHost {
	return s.soHost
}

// GetPrivKey returns the private key for signing invite messages.
func (s *SharedObject) GetPrivKey() crypto.PrivKey {
	return s.localPriv
}

// GetProviderID returns the provider identifier for the invite message.
func (s *SharedObject) GetProviderID() string {
	return s.tkr.a.t.accountInfo.GetProviderId()
}

// CreateSOInviteOp creates a signed invite and stores it locally.
func (s *SharedObject) CreateSOInviteOp(
	ctx context.Context,
	ownerPrivKey crypto.PrivKey,
	providerID string,
	terms *sobject.SOInvite,
) (*sobject.SOInviteMessage, error) {
	return s.soHost.CreateSOInviteOp(ctx, ownerPrivKey, providerID, terms)
}

// RevokeInvite revokes an invite locally.
func (s *SharedObject) RevokeInvite(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	return s.soHost.RevokeInvite(ctx, signerPrivKey, inviteID)
}

// IncrementInviteUses increments invite uses locally.
func (s *SharedObject) IncrementInviteUses(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error {
	return s.soHost.IncrementInviteUses(ctx, signerPrivKey, inviteID)
}

// SetRosterDropped drops exactly the writers in dropped from the trimming
// roster, signed by the local peer.
func (s *SharedObject) SetRosterDropped(ctx context.Context, dropped []string) (bool, error) {
	return sobject.SetSORoster(ctx, s.soHost, dropped, s.localPriv)
}

// SetSequencer appoints peerID as the sequencer, or selects Merge when peerID
// is empty, signed by the local peer.
func (s *SharedObject) SetSequencer(ctx context.Context, peerID string) (bool, error) {
	return sobject.SetSOSequencer(ctx, s.soHost, peerID, s.localPriv)
}

// SetControl changes who controls the shared object, signed or agreed to by
// the local peer.
func (s *SharedObject) SetControl(ctx context.Context, control sobject.SOControl) error {
	return sobject.SetSOControl(ctx, s.soHost, control, s.localPriv)
}

// ApproveConfigChange agrees, as the local peer, to a change another voter
// asked the group for.
func (s *SharedObject) ApproveConfigChange(ctx context.Context, hash []byte) error {
	return sobject.ApproveSOConfigChange(ctx, s.soHost, hash, s.localPriv)
}

// SequenceOperations places the unplaced operations while the local peer is
// the sequencer, so this device orders the Space as its main device.
func (s *SharedObject) SequenceOperations(ctx context.Context) error {
	return s.soHost.SequenceOperations(ctx, s.localPriv)
}

// _ verifies the local provider's SharedObject contracts.
var (
	_ sobject.SharedObjectHealthAccessor = (*SharedObject)(nil)
	_ sobject.SharedObjectProvider       = (*ProviderAccount)(nil)
	_ sobject.SharedObject               = (*SharedObject)(nil)
	_ sobject.InviteHost                 = (*SharedObject)(nil)
	_ sobject.RosterHost                 = (*SharedObject)(nil)
	_ sobject.SequencerHost              = (*SharedObject)(nil)
	_ sobject.ControlHost                = (*SharedObject)(nil)
	_ sobject.MainDevice                 = (*SharedObject)(nil)
)

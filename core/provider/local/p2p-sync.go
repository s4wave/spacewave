package provider_local

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	sobject_sync "github.com/s4wave/spacewave/core/sobject/sync"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// p2pSyncPhase is a sync generation's position in its lifecycle. The phase
// only moves forward. P2PSyncLifecycle.tla models the transitions with the
// replacements and owners around them.
type p2pSyncPhase uint8

const (
	// p2pSyncStarting runs the startup pass; reads fall back to lowerSource.
	p2pSyncStarting p2pSyncPhase = iota
	// p2pSyncRunning has started its controllers and follows the SO list.
	p2pSyncRunning
	// p2pSyncStopping is canceled; its run goroutine is releasing resources.
	p2pSyncStopping
	// p2pSyncStopped has released every resource.
	p2pSyncStopped
)

// p2pSyncState is one P2P sync generation: its lifecycle phase, its owners and
// the resources it releases when it stops. Its run goroutine, runP2PSync, owns
// the generation from startup to the release of its last resource. Other code
// reads the phase, adds resources and stops the generation through the methods
// below, each under bcast.
type p2pSyncState struct {
	// bcast guards the phase, owners, lowerSource and resource fields below.
	bcast broadcast.Broadcast

	// ctx carries cancellation across startup and running workers. It ends
	// exactly when the phase reaches Stopping.
	ctx context.Context
	// sessionTransport owns the child bus used by this sync generation.
	sessionTransport *transport.SessionTransport
	// childBus is immutable after readiness and retained through worker cleanup.
	childBus bus.Bus
	// cancel ends ctx; only stopLocked calls it.
	cancel context.CancelFunc

	// phase is the lifecycle position; only the methods below change it.
	phase p2pSyncPhase
	// startErr is why the generation left Starting without running, or nil.
	startErr error
	// owners counts callers and successor states retaining this generation.
	owners int
	// lowerSource is the generation this one replaces. This one owns it while
	// Starting, and hands it back for release on leaving Starting.
	lowerSource *p2pSyncState

	// refs holds controllerbus references until cleanup.
	refs []directive.Reference
	// peerRefs indexes retained peer directives whose references also appear in refs.
	peerRefs map[string]directive.Reference
	// relFns holds non-directive resource releases until cleanup.
	relFns []func()
	// stores indexes DEX stores by bucket ID for this generation.
	stores map[string]block.StoreOps
	// storeView mirrors stores and the predecessor reads fall back to, so
	// block reads resolve their store without bcast.
	storeView atomic.Pointer[p2pStoreView]
	// exchanges owns the traffic counters for each mounted DEX bucket.
	exchanges map[string]*dex_solicit.Controller
	// soSync indexes restartable shared-object sync routines.
	soSync map[string]*accountObjectSync
	// copyProgress records each Space's latest local durability check.
	copyProgress map[string]*AccountReplicaCopyState
	// replicaSync follows the canonical account catalog and Session membership.
	replicaSync *routine.RoutineContainer
	// udp dials the static peer addresses the listed objects carry.
	udp staticUDP
}

// newP2PSyncState builds a Starting generation on the ready transport, owned
// by its creating caller and holding lower, already retained, as the source its
// reads fall back to. Its context outlives ctx's cancellation.
func newP2PSyncState(
	ctx context.Context,
	sessionTransport *transport.SessionTransport,
	childBus bus.Bus,
	lower *p2pSyncState,
) *p2pSyncState {
	syncCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &p2pSyncState{
		ctx:              syncCtx,
		sessionTransport: sessionTransport,
		childBus:         childBus,
		cancel:           cancel,
		owners:           1,
		lowerSource:      lower,
	}
	s.publishStoreViewLocked()
	return s
}

// stopLocked moves a Starting or Running generation to Stopping and cancels
// ctx. A generation still Starting records context.Canceled as its start error
// unless startup already failed. The caller must hold bcast and broadcast.
func (s *p2pSyncState) stopLocked() {
	if s.phase >= p2pSyncStopping {
		return
	}
	if s.phase == p2pSyncStarting && s.startErr == nil {
		s.startErr = context.Canceled
	}
	s.phase = p2pSyncStopping
	s.cancel()
}

// stop moves a Starting or Running generation to Stopping. Its run goroutine
// then releases its resources.
func (s *p2pSyncState) stop() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.stopLocked()
		bcast()
	})
}

// retain adds an owner to a Starting or Running generation. It returns false
// once the generation is stopping.
func (s *p2pSyncState) retain() bool {
	var retained bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.phase >= p2pSyncStopping {
			return
		}
		s.owners++
		retained = true
		bcast()
	})
	return retained
}

// join attaches a caller on this generation's transport. A Running generation
// is reused without an owner: it already follows the SO list, and a later
// session mount must not drop its links and DEX streams. A Starting one gains
// ownerCtx's caller as an owner. joined is false once the generation is
// stopping or ownerCtx has ended.
func (s *p2pSyncState) join(ownerCtx context.Context) (joined, owned bool) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Join only a live generation for a live caller; own only a startup.
		if s.phase >= p2pSyncStopping || ownerCtx.Err() != nil {
			return
		}
		joined = true
		if s.phase == p2pSyncRunning {
			return
		}
		s.owners++
		owned = true
		bcast()
	})
	return joined, owned
}

// release drops one owner and stops the generation when it was the last. It
// reports whether this call stopped it.
func (s *p2pSyncState) release() bool {
	var stopped bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.owners == 0 {
			return
		}
		s.owners--
		if s.owners == 0 && s.phase < p2pSyncStopping {
			s.stopLocked()
			stopped = true
		}
		bcast()
	})
	return stopped
}

// finishStart ends the startup pass with err. On success the generation enters
// Running and returns its lower source for the caller to release, in the same
// step that stops reads from falling back to it. On failure it records err and
// stops. running is false when startup failed or the generation was stopped
// first.
func (s *p2pSyncState) finishStart(err error) (lower *p2pSyncState, running bool) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.phase != p2pSyncStarting {
			return
		}
		if err != nil {
			s.startErr = err
			s.stopLocked()
		} else {
			s.phase = p2pSyncRunning
			lower, s.lowerSource = s.lowerSource, nil
			s.publishStoreViewLocked()
			running = true
		}
		bcast()
	})
	return lower, running
}

// markStopped records that every resource has been released.
func (s *p2pSyncState) markStopped() {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.phase = p2pSyncStopped
		bcast()
	})
}

// getPhase returns the current lifecycle phase.
func (s *p2pSyncState) getPhase() p2pSyncPhase {
	var phase p2pSyncPhase
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		phase = s.phase
	})
	return phase
}

// waitPhase waits until the generation reaches phase or a later one, or
// returns ctx's error when ctx ends first.
func (s *p2pSyncState) waitPhase(ctx context.Context, phase p2pSyncPhase) error {
	for {
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if s.phase < phase {
				waitCh = getWaitCh()
			}
		})
		if waitCh == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// getStartErr returns why the generation left Starting without running, or nil.
func (s *p2pSyncState) getStartErr() error {
	var err error
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = s.startErr
	})
	return err
}

// syncRoutines returns every SO sync and replica routine. The run goroutine
// joins them at Stopping before releasing the resources they depend on; addSO
// refuses new routines by then.
func (s *p2pSyncState) syncRoutines() []*routine.RoutineContainer {
	var syncRoutines []*routine.RoutineContainer
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		syncRoutines = make([]*routine.RoutineContainer, 0, len(s.soSync)+1)
		for _, syncRoutine := range s.soSync {
			syncRoutines = append(syncRoutines, syncRoutine.sync)
			if syncRoutine.copy != nil {
				syncRoutines = append(syncRoutines, syncRoutine.copy)
			}
			if syncRoutine.hosting != nil {
				syncRoutines = append(syncRoutines, syncRoutine.hosting)
			}
		}
		if s.replicaSync != nil {
			syncRoutines = append(syncRoutines, s.replicaSync)
		}
	})
	return syncRoutines
}

// takeResources returns every reference, release function and the lower
// source, each once, and stops reads from falling back to the lower source.
// The run goroutine calls it at Stopping after joining the sync routines.
func (s *p2pSyncState) takeResources() (refs []directive.Reference, relFns []func(), lower *p2pSyncState) {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		refs, relFns, lower = s.refs, s.relFns, s.lowerSource
		s.refs, s.relFns, s.peerRefs, s.lowerSource = nil, nil, nil, nil
		s.publishStoreViewLocked()
	})
	return refs, relFns, lower
}

// p2pStoreView is an immutable copy of the stores a generation serves.
type p2pStoreView struct {
	// stores indexes DEX stores by bucket ID, never changed.
	stores map[string]block.StoreOps
	// lower serves buckets this generation lacks while it starts, or nil.
	lower *p2pSyncState
}

// publishStoreViewLocked republishes storeView after stores or lowerSource
// change. The caller must hold bcast, or own a state not yet published.
func (s *p2pSyncState) publishStoreViewLocked() {
	s.storeView.Store(&p2pStoreView{stores: maps.Clone(s.stores), lower: s.lowerSource})
}

// addStore registers the DEX store and exchange of bucketID.
func (s *p2pSyncState) addStore(bucketID string, ctrl *dex_solicit.Controller) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Index the store and its exchange, then republish the read view.
		if s.stores == nil {
			s.stores = make(map[string]block.StoreOps)
		}
		if s.exchanges == nil {
			s.exchanges = make(map[string]*dex_solicit.Controller)
		}
		s.stores[bucketID] = dex_solicit.NewStore(ctrl)
		s.exchanges[bucketID] = ctrl
		s.publishStoreViewLocked()
		bcast()
	})
}

// hasStore reports whether this generation has registered bucketID.
func (s *p2pSyncState) hasStore(bucketID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.stores[bucketID]
	})
	return ok
}

// hasSO reports whether this generation started sync for soID.
func (s *p2pSyncState) hasSO(soID string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.soSync[soID]
	})
	return ok
}

// addSO registers the sync routine for soID while the generation is Starting or
// Running.
func (s *p2pSyncState) addSO(soID string, syncRoutine *accountObjectSync) bool {
	var added bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Register the routine once, and only before the generation stops.
		if s.phase >= p2pSyncStopping {
			return
		}
		if s.soSync == nil {
			s.soSync = make(map[string]*accountObjectSync)
		}
		if _, exists := s.soSync[soID]; exists {
			return
		}
		s.soSync[soID] = syncRoutine
		added = true
		bcast()
	})
	return added
}

// restartSO replaces the sync routine of soID on a Running generation. It
// reports whether a routine was restarted.
func (s *p2pSyncState) restartSO(soID string) bool {
	var restarted bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if s.phase != p2pSyncRunning {
			return
		}
		if syncRoutine := s.soSync[soID]; syncRoutine != nil {
			restarted = syncRoutine.sync.RestartRoutine()
		}
	})
	return restarted
}

// addRef retains a controllerbus reference until state cleanup.
func (s *p2pSyncState) addRef(ref directive.Reference) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.refs = append(s.refs, ref)
		bcast()
	})
}

// addRelease retains a cleanup function until state cleanup.
func (s *p2pSyncState) addRelease(rel func()) {
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		s.relFns = append(s.relFns, rel)
		bcast()
	})
}

// hasPeerRef reports whether this generation retains a link to the peer with key.
func (s *p2pSyncState) hasPeerRef(key string) bool {
	var ok bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, ok = s.peerRefs[key]
	})
	return ok
}

// addPeerRef retains the link directive ref to the peer with key until cleanup.
// It returns false, leaving ref to the caller, when the generation is stopping
// or already retains the peer.
func (s *p2pSyncState) addPeerRef(key string, ref directive.Reference) bool {
	var added bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Retain one link per peer, and only before the generation stops.
		if s.phase >= p2pSyncStopping {
			return
		}
		if _, exists := s.peerRefs[key]; exists {
			return
		}
		if s.peerRefs == nil {
			s.peerRefs = make(map[string]directive.Reference)
		}
		s.peerRefs[key] = ref
		s.refs = append(s.refs, ref)
		added = true
		bcast()
	})
	return added
}

// takePeerRef removes the link to the peer with key and returns its reference
// for the caller to release, or nil when none is retained. The reference stays
// in refs, where a second release is a no-op.
func (s *p2pSyncState) takePeerRef(key string) directive.Reference {
	var ref directive.Reference
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ref = s.peerRefs[key]
		delete(s.peerRefs, key)
	})
	return ref
}

// getStore returns this generation's store or its startup predecessor's store.
func (s *p2pSyncState) getStore(bucketID string) block.StoreOps {
	// Every block read resolves its store here, so read the view without bcast.
	view := s.storeView.Load()
	if view == nil {
		return nil
	}
	if store := view.stores[bucketID]; store != nil {
		return store
	}
	if view.lower != nil {
		return view.lower.getStore(bucketID)
	}
	return nil
}

// getExchange returns the DEX controller of bucketID, or nil when none started.
func (s *p2pSyncState) getExchange(bucketID string) *dex_solicit.Controller {
	var exchange *dex_solicit.Controller
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		exchange = s.exchanges[bucketID]
	})
	return exchange
}

// getExchangesWithWait returns the DEX controller of every mounted bucket and a
// channel that closes when the generation changes.
func (s *p2pSyncState) getExchangesWithWait() ([]*dex_solicit.Controller, <-chan struct{}) {
	var exchanges []*dex_solicit.Controller
	var waitCh <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		exchanges = slices.Collect(maps.Values(s.exchanges))
		waitCh = getWaitCh()
	})
	return exchanges, waitCh
}

// watchP2PSyncOwner releases one state owner when ctx ends. A context without a
// cancellation channel leaves its owner retained until explicit retirement.
func (a *ProviderAccount) watchP2PSyncOwner(ctx context.Context, state *p2pSyncState) {
	if ctx.Done() == nil {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
			a.releaseP2PSyncState(state)
		case <-state.ctx.Done():
		}
	}()
}

// releaseP2PSyncState drops one owner and retires the state after the last one.
func (a *ProviderAccount) releaseP2PSyncState(state *p2pSyncState) {
	if state.release() {
		a.retireP2PSyncState(state)
	}
}

// StartP2PSync starts SO sync and DEX block exchange for all mounted
// shared objects. Called when a P2P-linked device connects.
//
// The method waits for sessionTransport readiness and starts sync controllers
// on its child bus.
func (a *ProviderAccount) StartP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
	return a.startP2PSync(ctx, ctx, sessionTransport)
}

// StartPersistentP2PSync starts P2P sync for the provider-account lifetime
// while bounding startup by ctx.
func (a *ProviderAccount) StartPersistentP2PSync(ctx context.Context, sessionTransport *transport.SessionTransport) error {
	if a.lifecycleCtx == nil {
		return errors.New("provider account lifecycle is unavailable")
	}
	return a.startP2PSync(ctx, a.lifecycleCtx, sessionTransport)
}

// startP2PSync selects or replaces the current sync generation. ctx bounds the
// readiness and startup wait; ownerCtx retains a new or in-progress generation.
func (a *ProviderAccount) startP2PSync(ctx, ownerCtx context.Context, sessionTransport *transport.SessionTransport) error {
	// Capture the ready transport bus for the whole sync generation.
	if err := sessionTransport.AwaitReady(ctx); err != nil {
		return err
	}
	childBus := sessionTransport.GetChildBus()
	if childBus == nil {
		return errors.New("session transport child bus is not ready")
	}

	// Join the generation on this transport, or publish its replacement, under
	// the account lock.
	var (
		previous         *p2pSyncState
		previousRetained bool
		state            *p2pSyncState
		created, owned   bool
	)
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Join the live generation on the same transport.
		previous = a.p2pSync
		if previous != nil && previous.sessionTransport == sessionTransport {
			var joined bool
			if joined, owned = previous.join(ownerCtx); joined {
				state = previous
				return
			}
		}
		if ownerCtx.Err() != nil {
			return
		}

		// Bind a replacement to the captured transport; it reads through a live
		// predecessor until it runs.
		var lower *p2pSyncState
		if previous != nil && previous.retain() {
			lower = previous
		}
		previousRetained = lower != nil
		state = newP2PSyncState(ctx, sessionTransport, childBus, lower)
		a.setP2PSyncLocked(state)
		bcast()
		created, owned = true, true
	})
	if state == nil {
		return ownerCtx.Err()
	}

	// Release each owner when its caller lifetime ends.
	if owned {
		a.watchP2PSyncOwner(ownerCtx, state)
	}

	// Startup belongs to every owner of the generation, not to the caller that
	// created it, so it runs on the generation's own goroutine.
	if created {
		go a.runP2PSync(state, previous, previousRetained)
	}
	return a.awaitP2PSyncStart(ctx, state)
}

// markP2PPendingEnrollPeer records a device peer recorded by SpaceLink
// approval whose invite has not been consumed yet. Auto-start skips these
// peers so the owner never dials a device that has not connected once in
// this process; the Device dials the owner through the one-use invite.
func (a *ProviderAccount) markP2PPendingEnrollPeer(remotePeerID peer.ID) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pPendingEnrollPeers == nil {
			a.p2pPendingEnrollPeers = make(map[string]struct{})
		}
		a.p2pPendingEnrollPeers[remotePeerID.String()] = struct{}{}
		bcast()
	})
}

// clearP2PPendingEnrollPeer removes a device peer from the pending-enrollment
// set after the device joined through its invite.
func (a *ProviderAccount) clearP2PPendingEnrollPeer(remotePeerID peer.ID) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		delete(a.p2pPendingEnrollPeers, remotePeerID.String())
		bcast()
	})
}

// RetainP2PPeer keeps an EstablishLinkWithPeer directive across P2P sync
// state restarts. Device enrollment calls it with the persisted invite owner
// so daemon restart reconnects without repeating the one-use invite.
func (a *ProviderAccount) RetainP2PPeer(ctx context.Context, remotePeerID peer.ID) error {
	// Record the peer for later generations, then retain it on the running one.
	if remotePeerID == "" {
		return errors.New("remote P2P peer ID is required")
	}
	var state *p2pSyncState
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pPeerIDs == nil {
			a.p2pPeerIDs = make(map[string]peer.ID)
		}
		a.p2pPeerIDs[remotePeerID.String()] = remotePeerID
		state = a.p2pSync
		bcast()
	})
	if state == nil {
		return errors.New("P2P sync is not running")
	}
	return a.retainP2PPeerOnState(ctx, state, remotePeerID)
}

// setHeldP2PPeers makes peers the set of peers holder keeps links to. It
// retains each peer as RetainP2PPeer does, across P2P sync state restarts, and
// releases the link to each peer holder named before and no longer names,
// unless RetainP2PPeer or another holder still names it. Empty peers drops the
// holder.
func (a *ProviderAccount) setHeldP2PPeers(ctx context.Context, holder string, peers []peer.ID) error {
	// Replace the holder's set, releasing the links nobody names any more. The
	// release runs under p2pSyncBcast, so a concurrent RetainP2PPeer either
	// names the peer first or finds its link gone and retains it again.
	var state *p2pSyncState
	var released []directive.Reference
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Store the holder's next set.
		next := make(map[string]peer.ID, len(peers))
		for _, id := range peers {
			next[id.String()] = id
		}
		prev := a.p2pHeldPeers[holder]
		if len(next) == 0 {
			delete(a.p2pHeldPeers, holder)
		} else {
			if a.p2pHeldPeers == nil {
				a.p2pHeldPeers = make(map[string]map[string]peer.ID)
			}
			a.p2pHeldPeers[holder] = next
		}

		// Release the link of each dropped peer that nobody else names.
		state = a.p2pSync
		for key := range prev {
			if _, kept := next[key]; kept || a.p2pPeerNamedLocked(key) || state == nil {
				continue
			}
			if ref := state.takePeerRef(key); ref != nil {
				released = append(released, ref)
			}
		}
		bcast()
	})
	for _, ref := range released {
		ref.Release()
	}
	if state == nil {
		return errors.New("P2P sync is not running")
	}

	// Retain the peers on the running generation.
	for _, id := range peers {
		if err := a.retainP2PPeerOnState(ctx, state, id); err != nil {
			return err
		}
	}
	return nil
}

// p2pPeerNamedLocked reports whether RetainP2PPeer or a holder names the peer
// with key. The caller must hold p2pSyncBcast.
func (a *ProviderAccount) p2pPeerNamedLocked(key string) bool {
	if _, ok := a.p2pPeerIDs[key]; ok {
		return true
	}
	for _, held := range a.p2pHeldPeers {
		if _, ok := held[key]; ok {
			return true
		}
	}
	return false
}

// p2pRetainedPeersLocked returns each peer RetainP2PPeer or a holder names,
// once. The caller must hold p2pSyncBcast.
func (a *ProviderAccount) p2pRetainedPeersLocked() []peer.ID {
	all := maps.Clone(a.p2pPeerIDs)
	for _, held := range a.p2pHeldPeers {
		if all == nil {
			all = make(map[string]peer.ID, len(held))
		}
		maps.Copy(all, held)
	}
	return slices.Collect(maps.Values(all))
}

// retainP2PPeerOnState retains one peer-link directive for the state lifetime.
func (a *ProviderAccount) retainP2PPeerOnState(
	ctx context.Context,
	state *p2pSyncState,
	remotePeerID peer.ID,
) error {
	// Reuse the local peer or a directive already held by this generation.
	if remotePeerID == state.sessionTransport.GetPeerID() {
		return nil
	}
	remoteKey := remotePeerID.String()
	if state.hasPeerRef(remoteKey) {
		return nil
	}

	// Keep an attached value reference as well as the directive reference.
	// A directive with no value handler leaves its MountedLink unreferenced, so
	// controllerbus disposes the link after EstablishLinkWithPeer's grace period.
	handler := directive.NewTypedCallbackHandler[link.MountedLink](
		func(directive.TypedAttachedValue[link.MountedLink]) {},
		nil,
		nil,
		nil,
	)

	// Attach peer demand to the bus captured at this generation's readiness.
	_, ref, err := state.childBus.AddDirective(
		ctx,
		link.NewEstablishLinkWithPeer(state.sessionTransport.GetPeerID(), remotePeerID),
		handler,
	)
	if err != nil {
		return err
	}

	// Retain the directive only while this generation still accepts work.
	if !state.addPeerRef(remoteKey, ref) {
		ref.Release()
	}
	return nil
}

// retainConfiguredP2PPeers retains every peer RetainP2PPeer or a holder
// names on state.
func (a *ProviderAccount) retainConfiguredP2PPeers(state *p2pSyncState) error {
	var peers []peer.ID
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		peers = a.p2pRetainedPeersLocked()
	})
	for _, remotePeerID := range peers {
		if err := a.retainP2PPeerOnState(state.ctx, state, remotePeerID); err != nil {
			return err
		}
	}
	return nil
}

// awaitP2PSyncStart waits for the shared startup to finish or for the caller's
// own context to end, whichever comes first. Giving up leaves the generation
// to its other owners; its watcher releases this caller's ownership.
func (a *ProviderAccount) awaitP2PSyncStart(ctx context.Context, state *p2pSyncState) error {
	// Wait for startup to end, and report its failure.
	if err := state.waitPhase(ctx, p2pSyncRunning); err != nil {
		return err
	}
	if err := state.getStartErr(); err != nil {
		return err
	}

	// Report a generation that is still current as started.
	var current bool
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = a.p2pSync == state
	})
	if current {
		return nil
	}
	if err := state.ctx.Err(); err != nil {
		return err
	}
	return errors.New("P2P sync stopped during startup")
}

// runP2PSync owns one generation from startup to the release of its last
// resource. It runs the startup pass until it has covered the current SO list,
// then either retires the predecessor and follows the SO list while Running,
// or restores the predecessor after a failed start.
func (a *ProviderAccount) runP2PSync(state, previous *p2pSyncState, previousRetained bool) {
	// Release the generation's resources however it ends.
	defer a.finishP2PSyncState(state)

	// Rerun the startup pass while the SO list changes under it.
	var inviteStarted bool
	soList := a.soListCtr.GetValue()
	err := a.startP2PSyncControllers(state, &inviteStarted, soList)
	for err == nil && a.soListCtr.GetValue() != soList {
		soList = a.soListCtr.GetValue()
		err = a.startP2PSyncControllers(state, &inviteStarted, soList)
	}
	lower, running := state.finishStart(err)
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		bcast()
	})
	if !running {
		a.restoreP2PSyncAfterFailedStart(state, previous, previousRetained)
		return
	}

	// The generation serves its own stores now; retire the predecessor.
	if lower != nil {
		a.releaseP2PSyncState(lower)
	}
	if previous != nil {
		a.retireP2PSyncState(previous)
	}

	// Follow the SO list until the generation stops.
	for {
		soList, err = a.soListCtr.WaitValueChange(state.ctx, soList, nil)
		if err != nil {
			return
		}
		if err := a.startP2PSyncControllers(state, &inviteStarted, soList); err != nil && state.ctx.Err() == nil {
			a.le.WithError(err).Warn("failed to synchronize added shared objects")
		}
	}
}

// finishP2PSyncState stops state and releases its resources once: it joins the
// sync routines, then releases the references, release functions and lower
// source in that order, and marks the generation Stopped. Only the
// generation's run goroutine calls it.
func (a *ProviderAccount) finishP2PSyncState(state *p2pSyncState) {
	// Stop the routines before releasing what they depend on.
	state.stop()
	for _, syncRoutine := range state.syncRoutines() {
		if exitedCh, _ := syncRoutine.SetRoutine(nil); exitedCh != nil {
			<-exitedCh
		}
	}
	refs, relFns, lower := state.takeResources()
	for _, ref := range refs {
		ref.Release()
	}
	for _, rel := range relFns {
		rel()
	}
	if lower != nil {
		a.releaseP2PSyncState(lower)
	}
	state.markStopped()
}

// restoreP2PSyncAfterFailedStart reconciles the account after state fails to
// start. While state is still current, a live predecessor it retained becomes
// current again; otherwise the predecessor is retired.
func (a *ProviderAccount) restoreP2PSyncAfterFailedStart(
	state *p2pSyncState,
	previous *p2pSyncState,
	previousRetained bool,
) {
	retirePrevious := false
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.p2pSync != state {
			return
		}
		if previousRetained && previous.getPhase() < p2pSyncStopping {
			a.setP2PSyncLocked(previous)
		} else {
			a.setP2PSyncLocked(nil)
			retirePrevious = previous != nil
		}
		bcast()
	})
	if retirePrevious {
		a.retireP2PSyncState(previous)
	}
}

// startP2PSyncControllers reconciles peer, DEX, shared-object, and invite
// controllers with soList for one sync generation.
func (a *ProviderAccount) startP2PSyncControllers(
	state *p2pSyncState,
	inviteStarted *bool,
	soList *sobject.SharedObjectList,
) error {
	// Reconcile the shared objects against this generation's retained bus.
	syncCtx, childBus := state.ctx, state.childBus
	state.removeMissingObjects(soList)
	a.dropMissingSOLinkHolders(syncCtx, soList)
	if err := a.retainConfiguredP2PPeers(state); err != nil {
		return errors.Wrap(err, "retain configured P2P peers")
	}
	a.reconcileStaticUDP(state, soList)
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		provRef := ref.GetProviderResourceRef()
		soID := provRef.GetId()
		blockStoreID := ref.GetBlockStoreId()

		providerID := provRef.GetProviderId()
		providerAccountID := provRef.GetProviderAccountId()
		bucketID := BlockStoreBucketID(providerID, providerAccountID, blockStoreID)
		if !state.hasStore(bucketID) {
			if err := a.startDEXSolicit(syncCtx, childBus, bucketID, soID, state); err != nil {
				if syncCtx.Err() != nil {
					return syncCtx.Err()
				}
				a.le.WithError(err).WithField("bucket-id", bucketID).Warn("failed to start dex solicit")
			}
		}

		if state.hasSO(soID) {
			continue
		}
		if err := a.startSOSync(syncCtx, childBus, ref, ReplicatesSpace(entry), soID, state); err != nil {
			if syncCtx.Err() != nil {
				return syncCtx.Err()
			}
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to start so sync")
			continue
		}
	}

	// Attach account services to the same bus captured at transport readiness.
	if !*inviteStarted {
		if err := a.startInviteServer(syncCtx, childBus, state.sessionTransport, state); err != nil {
			if syncCtx.Err() != nil {
				return syncCtx.Err()
			}
			a.le.WithError(err).Warn("failed to start invite server")
			return syncCtx.Err()
		}
		if err := a.startAccountReplicaSync(state); err != nil {
			return err
		}
		*inviteStarted = true
	}
	return syncCtx.Err()
}

// GetP2PSyncSnapshotWithWait returns whether P2P sync is running and a channel
// that closes when its lifecycle changes.
func (a *ProviderAccount) GetP2PSyncSnapshotWithWait() (bool, <-chan struct{}) {
	var (
		running bool
		ch      <-chan struct{}
	)
	a.p2pSyncBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		if a.p2pSync == nil {
			return
		}
		running = a.p2pSync.getPhase() == p2pSyncRunning
	})
	return running, ch
}

// IsP2PSyncRunning returns whether P2P sync is currently active.
// Safe to call from any goroutine.
func (a *ProviderAccount) IsP2PSyncRunning() bool {
	running, _ := a.GetP2PSyncSnapshotWithWait()
	return running
}

// RetrySharedObjectSync replaces the running SO sync directive for soID while
// preserving the active transport generation and every other controller. It
// reports whether an existing routine was restarted.
func (a *ProviderAccount) RetrySharedObjectSync(soID string) bool {
	if soID == "" {
		return false
	}

	// Hold both lifecycle locks through the restart decision so retirement or
	// generation replacement cannot redirect the request to a stale routine.
	var restarted bool
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state := a.p2pSync
		if state == nil {
			return
		}
		restarted = state.restartSO(soID)
	})
	return restarted
}

// StopP2PSync stops all P2P sync controllers, waits for goroutines
// to finish, and releases references.
func (a *ProviderAccount) StopP2PSync() {
	a.retireP2PSyncState(nil)
}

// serveStore is a mounted block store's reads for a peer's request.
type serveStore struct {
	block.StoreOps
}

// getServeStore returns the reads a peer's request for a block of bucketID
// takes, or nil while no block store of the bucket is mounted.
func (a *ProviderAccount) getServeStore(bucketID string) block.StoreOps {
	v, ok := a.serveStores.Load(bucketID)
	if !ok {
		return nil
	}
	return v.(*serveStore).StoreOps
}

// getP2PStore returns the active lifecycle's store for bucketID, including its
// retained predecessor while replacement startup is incomplete.
func (a *ProviderAccount) getP2PStore(bucketID string) block.StoreOps {
	// Every block read resolves its lower store here, so read the lifecycle
	// without p2pSyncBcast.
	state := a.p2pSyncCur.Load()
	if state == nil {
		return nil
	}
	return state.getStore(bucketID)
}

// setP2PSyncLocked replaces the current lifecycle and its lock-free mirror.
// The caller must hold p2pSyncBcast.
func (a *ProviderAccount) setP2PSyncLocked(state *p2pSyncState) {
	a.p2pSync = state
	a.p2pSyncCur.Store(state)
}

// waitP2PStore waits until a P2P sync lifecycle serves bucketID and returns
// its store, or returns nil when ctx ends first. It wakes when the account
// changes lifecycles and when the current one publishes its stores.
func (a *ProviderAccount) waitP2PStore(ctx context.Context, bucketID string) block.StoreOps {
	for {
		// Read the wakes of the next lifecycle change and store publication.
		var accountWait, stateWait <-chan struct{}
		var state *p2pSyncState
		a.p2pSyncBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			accountWait, state = getWait(), a.p2pSync
		})
		if state != nil {
			state.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
				stateWait = getWait()
			})
		}

		// Return the store once one serves the bucket, or wait for a change.
		if store := a.getP2PStore(bucketID); store != nil {
			return store
		}
		select {
		case <-ctx.Done():
			return nil
		case <-accountWait:
		case <-stateWait:
		}
	}
}

// retireP2PSyncState detaches state from the account, stops it, and waits for
// its run goroutine to release its resources. A nil state selects the current
// state atomically with removal. A generation's own run goroutine must not
// retire it, since it would wait on itself.
func (a *ProviderAccount) retireP2PSyncState(state *p2pSyncState) {
	a.p2pSyncBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if state == nil {
			state = a.p2pSync
		}
		if state != nil && a.p2pSync == state {
			a.setP2PSyncLocked(nil)
			bcast()
		}
	})
	if state == nil {
		return
	}
	state.stop()
	_ = state.waitPhase(context.Background(), p2pSyncStopped)
}

// ReplicatesSpace reports whether the account copies the Space in entry whole
// to its replica. A Space joined by invitation stays lazy: it fetches only the
// blocks a reader asks for.
func ReplicatesSpace(entry *sobject.SharedObjectListEntry) bool {
	return entry.GetMeta().GetBodyType() == space.SpaceBodyType && entry.GetSource() != "shared"
}

// startSOSync mounts the shared object and starts an SOSync instance for it.
// replicate also copies the object's blocks to the account replica.
func (a *ProviderAccount) startSOSync(
	ctx context.Context,
	childBus bus.Bus,
	ref *sobject.SharedObjectRef,
	replicate bool,
	soID string,
	state *p2pSyncState,
) (retErr error) {
	// Cancel every routine of this object when startup fails.
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()

	// Mount the SO to ensure the tracker is initialized with the ref.
	// This is necessary when StartP2PSync is called from auto-start
	// (before any UI-driven mount).
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	localSO := so.(*SharedObject)

	// Validate inbound snapshots against the local storage identity that holds
	// the Space grant. The session transport peer routes the stream but is not
	// necessarily a participant in a local Space.
	validateSnapshotAccess := func(ctx context.Context, state *sobject.SOState) error {
		snapshot := sobject.NewSOStateParticipantHandle(
			a.le,
			a.t.p.sfs,
			soID,
			state,
			localSO.localPriv,
			localSO.localPid,
		)
		_, err := snapshot.GetCheckpoint(ctx)
		return err
	}

	// Report peer admission, recovery, and connections to the object's health.
	soSync := sobject_sync.NewSOSync(
		a.le,
		childBus,
		soID,
		localSO.GetPeerID(),
		localSO.localPriv,
		localSO.soHost,
		func(remoteID peer.ID, accepted bool) {
			localSO.run.swapHealth(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
				return health.WithSyncPeerAdmission(remoteID.String(), accepted)
			})
		},
		validateSnapshotAccess,
	)
	soSync.SetPeerRecoveryObserver(func(remoteID peer.ID, required bool) {
		localSO.run.swapHealth(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
			return health.WithSyncPeerRecovery(remoteID.String(), required)
		})
	})
	soSync.SetPeerStateObserver(localSO.tkr.recordPeerState)
	soSync.SetPeerConnectionObserver(func(remoteID peer.ID, connected bool, err error) {
		localSO.run.swapHealth(func(health *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
			return health.WithSyncPeerConnection(remoteID.String(), connected, err, time.Now())
		})
	})

	// Retain the mount for this generation while allowing an explicit invite to
	// replace only the solicitation routine. RoutineContainer serializes the
	// replacement behind the prior execution's exit.
	syncRoutine := routine.NewRoutineContainer()
	syncRoutine.SetRoutine(func(runCtx context.Context) error {
		err := soSync.Execute(runCtx)
		if err != nil && runCtx.Err() == nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("so sync exited with error")
		}
		return err
	})

	// Move hosting with each ownership transfer.
	hostingRoutine := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-so-hosting"), routine.WithRetry(providerBackoff))
	hostingRoutine.SetRoutine(func(ctx context.Context) error { return a.watchSOHosting(ctx, soID, localSO) })
	objectSync := &accountObjectSync{sync: syncRoutine, hosting: hostingRoutine, cancel: cancel, release: relSO}

	// Copy a Space the account replicates alongside its sync.
	if replicate {
		objectSync.copy = routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-copy"), routine.WithRetry(providerBackoff))
		provRef := ref.GetProviderResourceRef()
		exchange := state.getExchange(BlockStoreBucketID(provRef.GetProviderId(), provRef.GetProviderAccountId(), ref.GetBlockStoreId()))
		objectSync.copy.SetRoutine(func(ctx context.Context) error {
			return a.runAccountReplicaCopy(ctx, so, state, exchange)
		})
	}

	// Register the object once; a concurrent start keeps the first registration.
	if !state.addSO(soID, objectSync) {
		relSO()
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.Errorf("shared object sync already started: %s", soID)
	}
	state.addRelease(relSO)
	state.addRelease(cancel)

	// Start every routine registered for the object.
	syncRoutine.SetContext(ctx, false)
	if objectSync.copy != nil {
		objectSync.copy.SetContext(ctx, false)
	}
	objectSync.hosting.SetContext(ctx, false)
	return nil
}

// startInviteServer registers the SO invite SRPC server on the child bus.
// The server handles incoming alpha/so-invite streams from invitees.
func (a *ProviderAccount) startInviteServer(ctx context.Context, childBus bus.Bus, st *transport.SessionTransport, state *p2pSyncState) error {
	// The invite server answers on the session transport peer.
	localPeerID := st.GetPeerID().String()

	// Build lookup function: scan all mounted SOs for matching token_hash.
	lookupFn := func(ctx context.Context, tokenHash []byte) (*sobject_invite.InviteLookupResult, error) {
		soList := a.soListCtr.GetValue()
		for _, entry := range soList.GetSharedObjects() {
			so, relSO, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
			if err != nil {
				continue
			}

			localSO, ok := so.(*SharedObject)
			if !ok {
				relSO()
				continue
			}

			soState, err := localSO.soHost.GetHostState(ctx)
			if err != nil {
				relSO()
				continue
			}

			for _, inv := range soState.GetInvites() {
				if bytes.Equal(inv.GetTokenHash(), tokenHash) {
					relSO()
					return a.inviteLookupResult(ctx, localSO, inv)
				}
			}
			relSO()
		}
		return nil, nil
	}

	// Build queue function: hold a redemption for the owner's approval.
	queueFn := func(ctx context.Context, result *sobject_invite.InviteLookupResult, joinResp, storageJoinResp *sobject.SOJoinResponse) error {
		// Mount the invite's shared object.
		so, relSO, err := a.mountListedSharedObject(ctx, result.SharedObjectID)
		if err != nil {
			return err
		}
		if so == nil {
			return errors.New("shared object is not held")
		}
		defer relSO()

		// Persist the request on the shared object.
		return so.QueueJoinRequest(ctx, joinResp, storageJoinResp)
	}

	// Build withdraw function: drop a requester's held redemption.
	withdrawFn := func(ctx context.Context, sharedObjectID string, peerID peer.ID) error {
		// Mount the shared object; one this account does not hold has no requests.
		so, relSO, err := a.mountListedSharedObject(ctx, sharedObjectID)
		if err != nil || so == nil {
			return err
		}
		defer relSO()

		// Remove the request, if one is still pending.
		return so.WithdrawJoinRequest(ctx, peerID.String())
	}

	// Build held config function: read the config of a Space this account holds.
	heldConfigFn := func(ctx context.Context, sharedObjectID string) (*sobject.SharedObjectConfig, error) {
		// Mount the shared object, or report nothing when it is not held.
		so, relSO, err := a.mountListedSharedObject(ctx, sharedObjectID)
		if err != nil || so == nil {
			return nil, err
		}
		defer relSO()

		// Read its current config from the host state.
		soState, err := so.soHost.GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		return soState.GetConfig(), nil
	}

	// Construct the invite controller with the handlers.
	ctrl, err := sobject_invite.NewInviteController(
		a.le,
		childBus,
		sobject_invite.Handlers{
			Lookup:     lookupFn,
			Enroll:     a.enrollInvitee,
			Leave:      a.acceptSharedObjectLeave,
			Queue:      queueFn,
			Withdraw:   withdrawFn,
			HeldConfig: heldConfigFn,
		},
		[]string{localPeerID},
	)
	if err != nil {
		return err
	}

	// Run the controller on the child bus until sync stops.
	relCtrl, err := childBus.AddController(ctx, ctrl, nil)
	if err != nil {
		return err
	}
	state.addRelease(relCtrl)
	return nil
}

// mountListedSharedObject mounts a shared object in the account's list by ID.
// Returns nil when the account does not hold the shared object.
func (a *ProviderAccount) mountListedSharedObject(ctx context.Context, sharedObjectID string) (*SharedObject, func(), error) {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		ref := entry.GetRef()
		if ref.GetProviderResourceRef().GetId() != sharedObjectID {
			continue
		}
		so, relSO, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			return nil, nil, err
		}
		localSO, ok := so.(*SharedObject)
		if !ok {
			relSO()
			return nil, nil, errors.New("shared object is not local")
		}
		return localSO, relSO, nil
	}
	return nil, nil, nil
}

// startDEXSolicit loads a DEX solicit controller on the child bus for
// the given block store bucket.
func (a *ProviderAccount) startDEXSolicit(ctx context.Context, childBus bus.Bus, bucketID, protocolContext string, state *p2pSyncState) error {
	// Start the controller and register its store and exchange for the bucket.
	ctrl, _, dexRef, err := loader.WaitExecControllerRunningTyped[*dex_solicit.Controller](
		ctx,
		childBus,
		resolver.NewLoadControllerWithConfig(&dex_solicit.Config{
			BucketId: bucketID,
			// Allow a participant to read writer blocks through its shared immediate
			// peer without requiring that peer to prefetch them.
			MaxForwardHops:  1,
			ProtocolContext: []byte(protocolContext),
		}),
		nil,
	)
	if err != nil {
		return err
	}
	ctrl.SetLocalSource(func() block.StoreOps { return a.getServeStore(bucketID) })
	state.addRef(dexRef)
	state.addStore(bucketID, ctrl)
	return nil
}

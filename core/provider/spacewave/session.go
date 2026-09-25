package provider_spacewave

import (
	"context"
	"crypto/rand"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// Session implements the session interface attached to sessionTracker.
type Session struct {
	// tkr links this handle to its mounted Session lifecycle.
	tkr *sessionTracker
	// objStore persists Session lock material and local indexes.
	objStore object.ObjectStore
	// stateAtomMgr manages shared session state atom stores.
	stateAtomMgr *resource_state.StateAtomManager
	// stateAtomStoreIndex tracks known session state atom store ids.
	stateAtomStoreIndex *session.StateAtomStoreIndex
	// sessionPriv signs as this Session and is nil while the Session is locked.
	sessionPriv crypto.PrivKey
	// sessionPid identifies sessionPriv on transports and cloud APIs.
	sessionPid peer.ID
	// storageKey encrypts auto-unlock material in objStore.
	storageKey [32]byte

	// lifecycleCtx owns direct transport mechanics for this mounted Session.
	lifecycleCtx context.Context

	// directP2PEnabled is the persisted Session-local transport policy.
	directP2PEnabled bool
	// pairingMu guards the engine for this unlocked Session lifetime.
	pairingMu     sync.Mutex
	pairingEngine *pairing.Engine
	pairingCancel context.CancelFunc
	// transitionWatcher follows account authority only while this Session is unlocked.
	transitionWatcher *routine.RoutineContainer
	// transportAuthWatcher repairs a rejected registration while this Session is unlocked.
	transportAuthWatcher *routine.RoutineContainer

	// lockTransition serializes startup and changes to the unlocked lifetime.
	lockTransition csync.Mutex

	// lockMode is the current lock mode, set during init and SetLockMode.
	lockMode session_lock.SessionLockMode
	// bcast guards lock state and policy changes.
	bcast broadcast.Broadcast
}

// GetBus returns the live Session child bus, falling back to the account bus.
func (s *Session) GetBus() bus.Bus {
	return s.tkr.a.getSessionBusForSession(s.tkr.id)
}

// GetSessionRef returns the ref to the session.
func (s *Session) GetSessionRef() *session.SessionRef {
	return &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                s.tkr.id,
			ProviderId:        s.tkr.a.p.info.GetProviderId(),
			ProviderAccountId: s.tkr.a.accountID,
		},
	}
}

// GetPeerId returns the peer id used by the session.
func (s *Session) GetPeerId() peer.ID {
	return s.sessionPid
}

// GetPrivKey returns the session private key.
// Returns nil if the session is locked.
func (s *Session) GetPrivKey() crypto.PrivKey {
	var priv crypto.PrivKey
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		priv = s.sessionPriv
	})
	return priv
}

// GetProviderAccount returns the handle to the session provider account.
func (s *Session) GetProviderAccount() provider.ProviderAccount {
	return s.tkr.a
}

// AccessStateAtomStore gets or creates a shared session state atom store.
func (s *Session) AccessStateAtomStore(ctx context.Context, storeID string) (resource_state.StateAtomStore, error) {
	// Resolve the shared state atom store through its lifecycle manager.
	store, err := s.stateAtomMgr.GetOrCreateStore(ctx, storeID)
	if err != nil {
		return nil, err
	}

	// Record the store ID in the Session-local discovery index.
	s.stateAtomStoreIndex.TrackStoreID(storeID)
	return store, nil
}

// SnapshotStateAtomStoreIDs returns the known session state atom store ids.
func (s *Session) SnapshotStateAtomStoreIDs(ctx context.Context) ([]string, error) {
	return s.stateAtomStoreIndex.SnapshotStoreIDs(ctx)
}

// WatchStateAtomStoreIDs watches the known session state atom store ids.
func (s *Session) WatchStateAtomStoreIDs(
	ctx context.Context,
	cb func(storeIDs []string) error,
) error {
	return s.stateAtomStoreIndex.WatchStoreIDs(ctx, cb)
}

// GetLockState returns the current lock mode and whether the session is locked.
func (s *Session) GetLockState(ctx context.Context) (session.SessionLockMode, bool, error) {
	// Read the persisted mode so the result reflects durable lock state.
	mode, err := session_lock.ReadLockMode(ctx, s.objStore, s.tkr.id)
	if err != nil {
		return 0, false, err
	}

	// Derive the active lock state from the in-memory private key.
	locked := s.GetPrivKey() == nil
	return session.SessionLockMode(mode), locked, nil
}

// GetDirectP2PEnabled returns the persisted Session-local transport policy.
func (s *Session) GetDirectP2PEnabled() bool {
	var enabled bool
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		enabled = s.directP2PEnabled
	})
	return enabled
}

// SetDirectP2PEnabled persists and reconciles the Session-local transport policy.
func (s *Session) SetDirectP2PEnabled(ctx context.Context, enabled bool) error {
	// Acquire the session controller and read mounted metadata.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.GetBus(), "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// Resolve the session metadata record.
	ref := s.GetSessionRef()
	meta, err := lookupSessionMetadata(ctx, sessionCtrl, ref)
	if err != nil {
		return err
	}

	// Fill metadata defaults for a first update.
	if meta == nil {
		meta = &session.SessionMetadata{
			ProviderDisplayName: "Cloud",
			ProviderId:          "spacewave",
		}
	}

	// Persist the direct-transport policy in session metadata.
	meta.DirectP2PDisabled = !enabled
	if err := sessionCtrl.UpdateSessionMetadata(ctx, ref, meta); err != nil {
		return err
	}

	// Publish the local policy and persist the account setting.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.directP2PEnabled = enabled
		broadcast()
	})
	return s.tkr.a.SetSessionDirectP2PEnabled(ctx, s.tkr.id, enabled)
}

// UnlockSession unlocks a PIN-locked session with the given PIN.
// No-op if the session is already unlocked.
func (s *Session) UnlockSession(ctx context.Context, pin []byte) error {
	release, err := s.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Require a locked Session configured for PIN encryption.
	if s.sessionPriv != nil {
		return nil
	}
	if s.lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		return errors.New("session is not PIN-locked")
	}

	// Read the encrypted PIN-lock material.
	encPriv, encSymKey, config, err := session_lock.ReadPINLockFiles(ctx, s.objStore, s.tkr.id)
	if err != nil {
		return errors.Wrap(err, "read PIN lock files")
	}
	if encPriv == nil || encSymKey == nil || config == nil {
		return errors.New("PIN lock files not found")
	}

	// Decrypt and parse the session private key.
	privPEM, err := session_lock.UnlockPIN(encPriv, encSymKey, config, pin)
	if err != nil {
		return err
	}

	privKey, err := keypem.ParsePrivKeyPem(privPEM)
	scrub.Scrub(privPEM)
	if err != nil {
		return errors.Wrap(err, "parse unlocked key")
	}

	// Install the key and reconcile authenticated transport state.
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.sessionPriv = privKey
	})
	s.tkr.a.maybeSetSessionClient(s.tkr.id, NewSessionClient(
		s.tkr.a.p.httpCli,
		s.tkr.a.p.endpoint,
		s.tkr.a.p.signingEnvPfx,
		privKey,
		s.sessionPid.String(),
	))
	if err := s.tkr.a.ConfigureSessionTransport(
		s.lifecycleCtx,
		s.tkr.id,
		privKey,
		s.tkr.a.p.endpoint,
		s.GetDirectP2PEnabled(),
	); err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		s.tkr.a.le.WithError(err).Warn("failed to reconcile session transport after unlock")
	}

	// Republish the retained Session and its unlocked transition lifetime.
	selfRef, _, _ := s.tkr.a.sessions.AddKeyRef(s.tkr.id)
	s.tkr.setPinnedRef(selfRef.Release)
	s.tkr.sessionProm.SetResult(s, nil)
	if s.transitionWatcher != nil {
		s.transitionWatcher.SetContext(s.lifecycleCtx, true)
	}
	if s.transportAuthWatcher != nil {
		s.transportAuthWatcher.SetContext(s.lifecycleCtx, true)
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return nil
}

// LockSession locks a running session, scrubbing the privkey from memory.
// Existing mounted references remain valid, but future mounts must wait for a
// fresh tracker run instead of reusing this in-memory session.
func (s *Session) LockSession(ctx context.Context) error {
	release, err := s.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Require a running PIN-encrypted Session before locking it.
	if s.lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		return errors.New("cannot lock: PIN mode not configured")
	}
	if s.sessionPriv == nil {
		return nil
	}

	s.clearPairingEngine()
	for _, watcher := range []*routine.RoutineContainer{s.transitionWatcher, s.transportAuthWatcher} {
		if watcher == nil {
			continue
		}
		watcher.ClearContext()
		if err := watcher.WaitExited(ctx, true, nil); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	// Scrub the private key from memory.
	raw, err := s.sessionPriv.Raw()
	if err == nil {
		scrub.Scrub(raw)
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.sessionPriv = nil
	})

	// Invalidate the published session and stop its transport.
	s.tkr.sessionProm.SetPromise(nil)
	s.tkr.unlockProm = promise.NewPromiseContainer[[]byte]()
	s.tkr.releasePinnedRef()
	s.tkr.a.dropSessionClientForSession(s.tkr.id)
	s.tkr.a.StopSessionTransportComposition(s.tkr.id)

	// Publish the locked session state.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return nil
}

// SetLockMode changes the session lock mode. Hot switch: session stays running.
func (s *Session) SetLockMode(ctx context.Context, mode session.SessionLockMode, pin []byte) error {
	release, err := s.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Require an unlocked Session before reading its private key.
	if s.sessionPriv == nil {
		return errors.New("session is locked")
	}

	// Serialize the private key for the selected lock representation.
	privPEM, err := keypem.MarshalPrivKeyPem(s.sessionPriv)
	if err != nil {
		return err
	}
	defer scrub.Scrub(privPEM)

	// Persist the selected auto-unlock or PIN-encrypted lock material.
	switch mode {
	case session.SessionLockMode_SESSION_LOCK_MODE_AUTO_UNLOCK:
		encPriv, err := session_lock.EncryptAutoUnlock(s.storageKey, privPEM)
		if err != nil {
			return err
		}
		if err := session_lock.WriteAutoUnlock(ctx, s.objStore, s.tkr.id, encPriv); err != nil {
			return err
		}
	case session.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED:
		encPriv, encSymKey, config, err := session_lock.CreatePINLock(privPEM, pin)
		if err != nil {
			return err
		}
		if err := session_lock.WritePINLock(ctx, s.objStore, s.tkr.id, encPriv, encSymKey, config); err != nil {
			return err
		}
	default:
		return nil
	}

	// Publish the new lock mode to live Session watchers.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.lockMode = session_lock.SessionLockMode(mode)
		broadcast()
	})

	// Update session metadata so the pre-mount PIN overlay hint stays current.
	s.updateSessionMetadata(ctx, mode)
	return nil
}

// updateSessionMetadata mirrors the current lock mode without replacing other
// Session presentation metadata.
func (s *Session) updateSessionMetadata(ctx context.Context, mode session.SessionLockMode) {
	// Acquire the controller that owns persisted Session metadata.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.GetBus(), "", false, nil)
	if err != nil {
		return
	}
	defer sessionCtrlRef.Release()

	// Read the current record so unrelated metadata fields remain intact.
	ref := s.GetSessionRef()
	meta, err := lookupSessionMetadata(ctx, sessionCtrl, ref)
	if err != nil {
		return
	}
	if meta == nil {
		meta = &session.SessionMetadata{
			ProviderDisplayName: "Cloud",
			ProviderId:          "spacewave",
		}
	}

	// Submit the best-effort lock-mode projection to the metadata owner.
	meta.LockMode = mode
	_ = sessionCtrl.UpdateSessionMetadata(ctx, ref, meta)
}

// lookupSessionMetadata resolves metadata for one Session reference.
func lookupSessionMetadata(
	ctx context.Context,
	ctrl session.SessionController,
	ref *session.SessionRef,
) (*session.SessionMetadata, error) {
	// Snapshot the registered Sessions from the metadata owner.
	entries, err := ctrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the matching entry through its controller-assigned index.
	for _, entry := range entries {
		if !entry.GetSessionRef().EqualVT(ref) {
			continue
		}
		return ctrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
	}
	return nil, nil
}

// directP2PEnabledFromMetadata applies the enabled-by-default transport policy.
func directP2PEnabledFromMetadata(meta *session.SessionMetadata) bool {
	return meta == nil || !meta.GetDirectP2PDisabled()
}

// WatchLockState calls the callback with the current lock state and on changes.
func (s *Session) WatchLockState(ctx context.Context, cb func(mode session.SessionLockMode, locked bool)) error {
	for {
		// Snapshot the lock state and the wait channel for its next change.
		var ch <-chan struct{}
		var mode session.SessionLockMode
		var locked bool
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			mode = session.SessionLockMode(s.lockMode)
			locked = s.sessionPriv == nil
		})

		// Publish the current state before waiting for another transition.
		cb(mode, locked)

		// Wait for cancellation or a state change without missing a broadcast.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// sessionTracker tracks a Session in the ProviderAccount.
type sessionTracker struct {
	// a is the provider account.
	a *ProviderAccount
	// id is the Session identifier.
	id string
	// ref is the reference to the session, set when instantiating the tracker.
	ref *promise.Promise[*session.SessionRef]
	// sessionProm is the session promise container.
	sessionProm *promise.PromiseContainer[*Session]
	// unlockProm is set when PIN-locked. Unblocks when UnlockSession is called.
	unlockProm *promise.PromiseContainer[[]byte]
	// pinnedRefMtx guards releasePinnedRefFn.
	pinnedRefMtx sync.Mutex
	// releasePinnedRefFn drops the self-ref that keeps the tracker alive.
	releasePinnedRefFn func()
}

// setPinnedRef installs the current self-ref release callback.
func (t *sessionTracker) setPinnedRef(release func()) {
	t.pinnedRefMtx.Lock()
	t.releasePinnedRefFn = release
	t.pinnedRefMtx.Unlock()
}

// releasePinnedRef drops the current self-ref, if any.
func (t *sessionTracker) releasePinnedRef() {
	// Detach the release callback while holding its guard.
	t.pinnedRefMtx.Lock()
	release := t.releasePinnedRefFn
	t.releasePinnedRefFn = nil
	t.pinnedRefMtx.Unlock()

	// Release outside the guard because the refcount may run lifecycle callbacks.
	if release != nil {
		release()
	}
}

// buildSessionTracker builds a new sessionTracker for a session id.
func (a *ProviderAccount) buildSessionTracker(sessionID string) (keyed.Routine, *sessionTracker) {
	tracker := &sessionTracker{
		a:           a,
		id:          sessionID,
		ref:         promise.NewPromise[*session.SessionRef](),
		sessionProm: promise.NewPromiseContainer[*Session](),
		unlockProm:  promise.NewPromiseContainer[[]byte](),
	}
	return tracker.executeSessionTracker, tracker
}

// executeSessionTracker executes the sessionTracker for the session.
func (t *sessionTracker) executeSessionTracker(rctx context.Context) (rerr error) {
	// Clear the prior result and publish any terminal mount failure.
	t.sessionProm.SetPromise(nil)
	defer func() {
		if rerr != nil && rerr != context.Canceled {
			t.sessionProm.SetResult(nil, rerr)
		}
	}()

	// Bind mounted resources and transports to this tracker execution.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Attach the Session identity to lifecycle logs.
	le := t.a.le.WithField("session-id", t.id)
	le.Debug("mounting session")

	// Wait for the ref.
	sessionRef, err := t.ref.Await(ctx)
	if err != nil {
		return err
	}

	// Resolve the account and Session storage identifiers.
	provRef := sessionRef.GetProviderResourceRef()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	// Mount ObjectStore in the account volume for session key storage.
	volID := t.a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx, t.a.p.b, false, objectStoreID, volID, ctxCancel,
	)
	if err != nil {
		return errors.Wrap(err, "mounting session object store")
	}
	defer diRef.Release()

	objStore := objStoreHandle.GetObjectStore()

	// Derive storage key from volume peer key.
	volPeer, err := t.a.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPrivKey, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume priv key")
	}
	storageKey, err := session_lock.DeriveStorageKey(volPrivKey)
	if err != nil {
		return errors.Wrap(err, "derive storage key")
	}

	// Check lock mode.
	lockMode, err := session_lock.ReadLockMode(ctx, objStore, sessionID)
	if err != nil {
		return errors.Wrap(err, "read lock mode")
	}

	// Load or create the private key according to the persisted lock mode.
	var privPEM []byte
	var sessionPriv crypto.PrivKey

	if lockMode == session_lock.SessionLockMode_PIN_ENCRYPTED {
		// Block until unlock RPC is called.
		le.Debug("session is PIN-locked, waiting for unlock")
		privPEM, err = t.unlockProm.Await(ctx)
		if err != nil {
			return err
		}
		sessionPriv, err = keypem.ParsePrivKeyPem(privPEM)
		scrub.Scrub(privPEM)
		if err != nil {
			return err
		}
	} else {
		// Auto-unlock: read and decrypt.
		data, found, err := session_lock.ReadAutoUnlockKey(ctx, objStore, sessionID)
		if err != nil {
			return errors.Wrap(err, "read auto-unlock key")
		}

		if found {
			privPEM, err = session_lock.DecryptAutoUnlock(storageKey, data)
			if err != nil {
				return errors.Wrap(err, "decrypt auto-unlock key")
			}
			sessionPriv, err = keypem.ParsePrivKeyPem(privPEM)
			scrub.Scrub(privPEM)
			if err != nil {
				return err
			}
		} else {
			// Generate new Ed25519 key.
			le.Debug("initializing session priv key")
			sessionPriv, _, err = crypto.GenerateEd25519Key(rand.Reader)
			if err != nil {
				return err
			}
			privPEM, err = keypem.MarshalPrivKeyPem(sessionPriv)
			if err != nil {
				return err
			}
			defer scrub.Scrub(privPEM)

			encPriv, err := session_lock.EncryptAutoUnlock(storageKey, privPEM)
			if err != nil {
				return err
			}
			if err := session_lock.WriteAutoUnlock(ctx, objStore, sessionID, encPriv); err != nil {
				return errors.Wrap(err, "write auto-unlock key")
			}
		}
	}

	// Derive the transport identity from the unlocked private key.
	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return err
	}

	le.WithField("sess-peer-id", sessionPeerID.String()).Debug("loaded session peer")

	// Check if this session peer ID is already registered with the cloud.
	regKey := []byte(sessionID + "/registered")
	registered := false
	if err := func() error {
		err := kvtx.RunTransaction(ctx, false,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, false)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				data, found, err := tx.Get(ctx, regKey)
				if err != nil {
					return err
				}
				registered = found && string(data) == sessionPeerID.String()
				return nil
			},
		)
		return err
	}(); err != nil {
		return errors.Wrap(err, "check session registration state")
	}

	// Register identities that have not yet been accepted for this key.
	var registeredObserved *api.ObservedSessionMetadata
	if !registered {
		observed, err := t.registerSession(ctx, objStore, sessionPriv, sessionPeerID)
		if err != nil {
			return err
		}
		registeredObserved = observed
	}

	// Keep the account signer pinned to the first mounted session unless this
	// tracker already owns the signer slot.
	t.a.maybeSetSessionClient(t.id, NewSessionClient(
		t.a.p.httpCli,
		t.a.p.endpoint,
		t.a.p.signingEnvPfx,
		sessionPriv,
		sessionPeerID.String(),
	))
	if !registered {
		t.a.bumpSelfRejoinSweepGeneration()
	}

	// Load the persisted direct-transport policy from the metadata owner.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, t.a.p.b, "", false, nil)
	if err != nil {
		return errors.Wrap(err, "lookup session metadata owner")
	}
	sessionMeta, err := lookupSessionMetadata(ctx, sessionCtrl, sessionRef)
	sessionCtrlRef.Release()
	if err != nil {
		return errors.Wrap(err, "load direct P2P policy")
	}
	directP2PEnabled := directP2PEnabledFromMetadata(sessionMeta)

	// Create the Session once. Lock/unlock mutates its fields in place
	// so existing references (MountSession directive, SessionResource)
	// always see the current state.
	stateAtomMgr := resource_state.NewStateAtomManager(t.a.p.b, objectStoreID, volID)
	defer stateAtomMgr.Release()
	so := &Session{
		tkr:                 t,
		objStore:            objStore,
		stateAtomMgr:        stateAtomMgr,
		stateAtomStoreIndex: session.NewStateAtomStoreIndex(objStore),
		lifecycleCtx:        ctx,
		directP2PEnabled:    directP2PEnabled,
		sessionPriv:         sessionPriv,
		sessionPid:          sessionPeerID,
		storageKey:          storageKey,
		lockMode:            lockMode,
	}

	so.transitionWatcher = routine.NewRoutineContainerWithLogger(le.WithField("routine", "account-transition"), routine.WithRetry(providerBackoff))
	so.transitionWatcher.SetRoutine(so.watchAccountTransition)
	so.transportAuthWatcher = routine.NewRoutineContainerWithLogger(le.WithField("routine", "transport-authorization"), routine.WithRetry(providerBackoff))
	so.transportAuthWatcher.SetRoutine(so.watchTransportAuthorization)

	// Retain account storage before publishing the Session to callers.
	accountRef, _, _ := t.a.p.accountRc.AddKeyRef(t.a.accountID)
	defer accountRef.Release()

	// Publishing permits account initialization to use this Session signer.
	// Locking must wait until startup has finished installing its workers.
	releaseStartup, err := so.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer releaseStartup()

	// Take a self-ref to keep the tracker alive even when all external
	// refs are released (e.g., between CLI commands).
	// This must happen before publishing sessionProm so callers cannot drop the
	// last external ref in the small window before the tracker pins itself.
	selfRef, _, _ := t.a.sessions.AddKeyRef(t.id)
	t.setPinnedRef(selfRef.Release)
	t.sessionProm.SetResult(so, nil)
	defer t.sessionProm.SetPromise(nil)
	defer t.releasePinnedRef()

	defer t.a.StopSessionTransportComposition(t.id)
	transitionWatcher := so.transitionWatcher
	transitionWatcher.SetContext(ctx, false)
	defer func() {
		if exited, _ := transitionWatcher.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()

	// Start direct transport. The authorization watcher repairs a rejected
	// registration whenever the cloud refuses the Session credential.
	if err := t.a.ConfigureSessionTransport(
		ctx,
		t.id,
		sessionPriv,
		t.a.p.endpoint,
		directP2PEnabled,
	); err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		le.WithError(err).Warn("failed to reconcile session transport")
	}
	transportAuthWatcher := so.transportAuthWatcher
	transportAuthWatcher.SetContext(ctx, false)
	defer func() {
		if exited, _ := transportAuthWatcher.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()

	releaseStartup()

	// Presentation writes may wait for account settings replication. Direct
	// transport and lock transitions must already be available while they wait.
	if registeredObserved != nil {
		if err := t.a.UpsertSessionPresentation(ctx, sessionPeerID.String(), registeredObserved); err != nil {
			le.WithError(err).Warn("failed to mirror session presentation metadata")
		}
	}

	// Keep mounted resources alive until the tracker lifecycle ends.
	<-ctx.Done()
	return context.Canceled
}

// UnlockPINSession unlocks a PIN-locked session before it is mounted.
// Decrypts the session key with the PIN and unblocks the tracker.
func (a *ProviderAccount) UnlockPINSession(ctx context.Context, ref *session.SessionRef, pin []byte) error {
	// Validate the provider reference before resolving Session storage.
	if err := ref.Validate(); err != nil {
		return err
	}

	// Resolve the account and Session identifiers used by the lock store.
	provRef := ref.GetProviderResourceRef()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	// Build the object store to read PIN lock files.
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store for unlock")
	}
	defer diRef.Release()
	objStore := objStoreHandle.GetObjectStore()

	// Read PIN lock files.
	encPriv, encSymKey, config, err := session_lock.ReadPINLockFiles(ctx, objStore, sessionID)
	if err != nil {
		return errors.Wrap(err, "read PIN lock files")
	}
	if encPriv == nil || encSymKey == nil || config == nil {
		return errors.New("session is not PIN-locked")
	}

	// Decrypt the session private key with the PIN.
	privPEM, err := session_lock.UnlockPIN(encPriv, encSymKey, config, pin)
	if err != nil {
		return err
	}

	// Unblock the tracker waiting on unlockProm.
	defer scrub.Scrub(privPEM)
	tkrRef, tkr, _ := a.sessions.AddKeyRef(sessionID)
	defer tkrRef.Release()
	tkr.ref.SetResult(ref, nil)
	tkr.unlockProm.SetResult(slices.Clone(privPEM), nil)

	// Retain the unlock until the Session has installed its own lifetime pin.
	_, err = tkr.sessionProm.Await(ctx)
	return err
}

// GetPINSessionRecoveryState reports that cloud PIN reset can use the account
// credential recovery path.
func (a *ProviderAccount) GetPINSessionRecoveryState(ctx context.Context, ref *session.SessionRef) (session.SessionRecoveryState, error) {
	// Validate the Session reference and advertise account recovery.
	if err := ref.Validate(); err != nil {
		return session.SessionRecoveryState_SESSION_RECOVERY_STATE_UNKNOWN, err
	}
	return session.SessionRecoveryState_SESSION_RECOVERY_STATE_AVAILABLE, nil
}

// ResetPINSession resets a PIN-locked session by deleting all lock files
// and the stored key. The session tracker will generate a fresh key on
// next mount.
func (a *ProviderAccount) ResetPINSession(ctx context.Context, ref *session.SessionRef, _ *session.EntityCredential) error {
	// Validate the provider reference before deleting Session lock material.
	if err := ref.Validate(); err != nil {
		return err
	}

	// Resolve the account and Session identifiers used by the lock store.
	provRef := ref.GetProviderResourceRef()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	// Mount the Session object store that owns lock and registration records.
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store for reset")
	}
	defer diRef.Release()
	objStore := objStoreHandle.GetObjectStore()

	// Delete all lock files and the stored key.
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			_ = tx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixPK))
			_ = tx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLocked))
			_ = tx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLockKey))
			_ = tx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLockParams))
			_ = tx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope))
			_ = tx.Delete(ctx, []byte(sessionID+"/registered"))
			return nil
		},
	)
	if err != nil {
		return errors.Wrap(err, "delete lock files")
	}

	// Restart the tracker so it generates a fresh key.
	a.sessions.RemoveKey(sessionID)

	return nil
}

// MountSession attempts to mount a Session returning the session and a release function.
func (a *ProviderAccount) MountSession(ctx context.Context, ref *session.SessionRef, released func()) (session.Session, func(), error) {
	// Validate the provider reference before acquiring its tracker.
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

	// Acquire the keyed tracker that exclusively owns this Session lifecycle.
	sessionID := ref.GetProviderResourceRef().GetId()
	tkrRef, tkr, _ := a.sessions.AddKeyRef(sessionID)

	// Set the ref in the tracker if not set.
	tkr.ref.SetResult(ref, nil)

	// Await the session handle to be ready.
	ws, err := tkr.sessionProm.Await(ctx)
	if err != nil {
		tkrRef.Release()
		return nil, nil, err
	}

	// Return the mounted Session with the caller's tracker release callback.
	return ws, tkrRef.Release, nil
}

// _ is a type assertion
var (
	_ session.SessionProvider = (*ProviderAccount)(nil)
	_ session.Session         = (*Session)(nil)
)

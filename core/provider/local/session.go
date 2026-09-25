package provider_local

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
	ctx context.Context
	tkr *sessionTracker

	objStore object.ObjectStore
	// stateAtomMgr manages shared session state atom stores.
	stateAtomMgr *resource_state.StateAtomManager
	// stateAtomStoreIndex tracks known session state atom store ids.
	stateAtomStoreIndex *session.StateAtomStoreIndex
	sessionPriv         crypto.PrivKey
	sessionPid          peer.ID
	storageKey          [32]byte

	// pairingMu guards the engine for this unlocked Session lifetime.
	pairingMu     sync.Mutex
	pairingEngine *pairing.Engine
	pairingCancel context.CancelFunc

	// transitionWatcher follows account redirects only while unlocked.
	transitionWatcher *routine.RoutineContainer

	// lockTransition serializes startup and changes to the unlocked lifetime.
	lockTransition csync.Mutex

	// lockMode is the current lock mode, set during init and SetLockMode.
	lockMode session_lock.SessionLockMode
	// bcast guards lock state changes.
	bcast broadcast.Broadcast
}

// GetBus returns the bus used for the session.
func (s *Session) GetBus() bus.Bus {
	return s.tkr.a.t.p.b
}

// GetSessionRef returns the ref to the session.
func (s *Session) GetSessionRef() *session.SessionRef {
	return &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                s.tkr.id,
			ProviderId:        s.tkr.a.t.p.info.GetProviderId(),
			ProviderAccountId: s.tkr.a.t.accountInfo.GetProviderAccountId(),
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
	store, err := s.stateAtomMgr.GetOrCreateStore(ctx, storeID)
	if err != nil {
		return nil, err
	}
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
	mode, err := session_lock.ReadLockMode(ctx, s.objStore, s.tkr.id)
	if err != nil {
		return 0, false, err
	}
	locked := s.GetPrivKey() == nil
	return session.SessionLockMode(mode), locked, nil
}

// UnlockSession unlocks a PIN-locked session. No-op if already unlocked.
func (s *Session) UnlockSession(ctx context.Context, pin []byte) error {
	release, err := s.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if s.sessionPriv != nil {
		return nil
	}
	if s.lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		return errors.New("session is not PIN-locked")
	}

	encPriv, encSymKey, config, err := session_lock.ReadPINLockFiles(ctx, s.objStore, s.tkr.id)
	if err != nil {
		return errors.Wrap(err, "read PIN lock files")
	}
	if encPriv == nil || encSymKey == nil || config == nil {
		return errors.New("PIN lock files not found")
	}

	privPEM, err := session_lock.UnlockPIN(encPriv, encSymKey, config, pin)
	if err != nil {
		return err
	}
	defer scrub.Scrub(privPEM)

	privKey, err := keypem.ParsePrivKeyPem(privPEM)
	if err != nil {
		return errors.Wrap(err, "parse unlocked key")
	}

	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.sessionPriv = privKey
	})

	selfRef, _, _ := s.tkr.a.sessions.AddKeyRef(s.tkr.id)
	s.tkr.setPinnedRef(selfRef.Release)
	s.tkr.sessionProm.SetResult(s, nil)
	if s.transitionWatcher != nil {
		s.transitionWatcher.SetContext(s.ctx, true)
	}

	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return s.tkr.startTransport(ctx, privKey)
}

// LockSession locks a running session, scrubbing the privkey from memory.
// The session tracker will restart and enter PIN-wait state when re-mounted.
func (s *Session) LockSession(ctx context.Context) error {
	release, err := s.lockTransition.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if s.lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		return errors.New("cannot lock: PIN mode not configured")
	}
	if s.sessionPriv == nil {
		return nil
	}

	s.clearPairingEngine()
	if s.transitionWatcher != nil {
		s.transitionWatcher.ClearContext()
		if err := s.transitionWatcher.WaitExited(ctx, true, nil); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	// End authenticated transport use before clearing the unlocked credential.
	if err := s.tkr.a.stopSessionPeerTransport(ctx, s.sessionPid); err != nil {
		return err
	}

	// Scrub the private key from memory.
	raw, err := s.sessionPriv.Raw()
	if err == nil {
		scrub.Scrub(raw)
	}
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.sessionPriv = nil
	})

	// Invalidate the session so new mounts trigger a tracker restart.
	s.tkr.sessionProm.SetPromise(nil)
	s.tkr.unlockProm = promise.NewPromiseContainer[[]byte]()
	s.tkr.releasePinnedRef()

	// Broadcast locked=true so WatchLockState emits.
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

	if s.sessionPriv == nil {
		return errors.New("session is locked")
	}
	privPEM, err := keypem.MarshalPrivKeyPem(s.sessionPriv)
	if err != nil {
		return err
	}
	defer scrub.Scrub(privPEM)

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

	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.lockMode = session_lock.SessionLockMode(mode)
		broadcast()
	})

	// Update session metadata so the pre-mount PIN overlay hint stays current.
	s.updateSessionMetadata(ctx, mode)
	return nil
}

// updateSessionMetadata updates the session controller metadata with the
// current lock mode. It reads existing metadata first to preserve other fields.
func (s *Session) updateSessionMetadata(ctx context.Context, mode session.SessionLockMode) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.GetBus(), "", true, nil)
	if err != nil || sessionCtrl == nil {
		return
	}
	defer sessionCtrlRef.Release()

	_ = updateSessionLockModeMetadata(ctx, sessionCtrl, s.GetSessionRef(), mode)
}

// updateSessionLockModeMetadata mirrors the lock mode without replacing the
// stable identity that keeps a mounted Session attached.
func updateSessionLockModeMetadata(
	ctx context.Context,
	ctrl session.SessionController,
	ref *session.SessionRef,
	mode session.SessionLockMode,
) error {
	meta, err := lookupSessionMetadata(ctx, ctrl, ref)
	if err != nil {
		return err
	}
	if meta == nil {
		providerRef := ref.GetProviderResourceRef()
		meta = &session.SessionMetadata{
			ProviderDisplayName: "Local",
			ProviderId:          providerRef.GetProviderId(),
			ProviderAccountId:   providerRef.GetProviderAccountId(),
		}
	}
	meta.LockMode = mode
	return ctrl.UpdateSessionMetadata(ctx, ref, meta)
}

// lookupSessionMetadata resolves metadata for one Session reference.
func lookupSessionMetadata(
	ctx context.Context,
	ctrl session.SessionController,
	ref *session.SessionRef,
) (*session.SessionMetadata, error) {
	entries, err := ctrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.GetSessionRef().EqualVT(ref) {
			continue
		}
		return ctrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
	}
	return nil, nil
}

// WatchLockState calls the callback with the current lock state and on changes.
func (s *Session) WatchLockState(ctx context.Context, cb func(mode session.SessionLockMode, locked bool)) error {
	for {
		var ch <-chan struct{}
		var mode session.SessionLockMode
		var locked bool
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			mode = session.SessionLockMode(s.lockMode)
			locked = s.sessionPriv == nil
		})
		cb(mode, locked)

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
	// id is the session ID.
	id string
	// cloudAccountID links this local session to a cloud account (empty for standalone).
	cloudAccountID string
	// seedPEM is an optional session private key PEM used only when the
	// session has no stored key yet. An existing stored key wins so the
	// session reopens with its durable identity.
	seedPEM []byte
	// ref is the reference set when instantiating the tracker.
	ref *promise.Promise[*session.SessionRef]
	// sessionProm is the session promise container.
	sessionProm *promise.PromiseContainer[*Session]
	// unlockProm is set when PIN-locked. Unblocks when UnlockSession is called.
	unlockProm *promise.PromiseContainer[[]byte]
	// pinnedRefMtx guards the reference retaining this unlocked Session.
	pinnedRefMtx       sync.Mutex
	releasePinnedRefFn func()
}

// setPinnedRef retains the tracker across temporary mount handoffs.
func (t *sessionTracker) setPinnedRef(release func()) {
	t.pinnedRefMtx.Lock()
	previous := t.releasePinnedRefFn
	t.releasePinnedRefFn = release
	t.pinnedRefMtx.Unlock()
	if previous != nil {
		previous()
	}
}

// releasePinnedRef releases the unlocked lifetime outside the reference guard.
func (t *sessionTracker) releasePinnedRef() {
	t.pinnedRefMtx.Lock()
	release := t.releasePinnedRefFn
	t.releasePinnedRefFn = nil
	t.pinnedRefMtx.Unlock()
	if release != nil {
		release()
	}
}

// startTransport starts the account's session transport and restores P2P
// sync for accounts with paired devices. Callers publish the Session first:
// transport readiness waits on the signaling ticket, which stalls until the
// startup deadline when the signaling endpoint is unreachable, and local work
// must not wait on the network. Returns only context.Canceled; other failures
// are logged.
func (t *sessionTracker) startTransport(ctx context.Context, sessionPriv crypto.PrivKey) error {
	relay := cloudRelayEndpoint{}
	if t.cloudAccountID != "" {
		relay = t.a.lookupCloudRelayEndpoint(ctx)
	}
	if relay.url == "" {
		relay = t.a.fallbackSignalingEndpoint()
	}

	le := t.a.le.WithField("session-id", t.id)
	_, _, err := t.a.ensureSessionTransportWithOwner(ctx, t.a.lifecycleCtx, sessionPriv, relay.url, relay.signingEnvPrefix, false)
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if err != nil {
		le.WithError(err).Warn("failed to start session transport")
		return nil
	}
	if st := t.a.GetSessionTransport(); st != nil {
		if err := t.a.AutoStartP2PSyncIfNeeded(ctx, st); err != nil {
			le.WithError(err).Warn("failed to auto-start P2P sync")
		}
	}
	return nil
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
	// Clear old state if any.
	t.sessionProm.SetPromise(nil)
	defer func() {
		if rerr != nil && rerr != context.Canceled {
			t.sessionProm.SetResult(nil, rerr)
		}
	}()

	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := t.a.le.WithField("session-id", t.id)
	le.Debug("mounting session")

	// Wait for the ref.
	sessionRef, err := t.ref.Await(ctx)
	if err != nil {
		return err
	}

	provRef := sessionRef.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	// Mount ObjectStore in the account volume for session key storage.
	volID := t.a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerID, providerAccountID)
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

	le.Debug("mounted object store for session successfully")

	objStore := objStoreHandle.GetObjectStore()

	// Derive storage key from volume peer key.
	volPeer, err := t.a.vol.GetPeer(ctx, true)
	if err != nil {
		return err
	}
	volPrivKey, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return err
	}
	storageKey, err := session_lock.DeriveStorageKey(volPrivKey)
	if err != nil {
		return err
	}

	// Check lock mode.
	lockMode, err := session_lock.ReadLockMode(ctx, objStore, sessionID)
	if err != nil {
		return err
	}
	if t.cloudAccountID == "" {
		var linkedCloudAccountID string
		err := kvtx.RunTransaction(ctx, false,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, false)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				data, found, err := tx.Get(ctx, LinkedCloudKey(sessionID))
				if err != nil {
					return err
				}
				if found && len(data) != 0 {
					linkedCloudAccountID = string(data)
				}
				return nil
			},
		)
		if err != nil {
			return err
		}
		t.cloudAccountID = linkedCloudAccountID
	}

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
		if err != nil {
			return err
		}
	}
	if lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		// Auto-unlock: read and decrypt.
		data, found, err := session_lock.ReadAutoUnlockKey(ctx, objStore, sessionID)
		if err != nil {
			return err
		}

		if found {
			privPEM, err = session_lock.DecryptAutoUnlock(storageKey, data)
			if err != nil {
				return err
			}
			sessionPriv, err = keypem.ParsePrivKeyPem(privPEM)
			if err != nil {
				return err
			}
		}
		if !found {
			// Generate new key (first time), or seed from the supplied key.
			le.Debug("initializing session priv key")
			seedPEM := t.seedPEM
			if len(seedPEM) == 0 {
				// The account volume can restart before the caller re-opens
				// the session; the provider-level seed keeps the identity.
				seedPEM = t.a.t.p.getSeedKey(providerID, providerAccountID, sessionID)
			}
			if len(seedPEM) != 0 {
				sessionPriv, err = keypem.ParsePrivKeyPem(seedPEM)
				if err != nil {
					return errors.Wrap(err, "parse session seed key")
				}
			} else {
				sessionPriv, _, err = crypto.GenerateEd25519Key(rand.Reader)
				if err != nil {
					return err
				}
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
				return err
			}

			// Write linked-cloud cross-reference if this session is linked to a cloud account.
			if t.cloudAccountID != "" {
				_ = kvtx.RunTransaction(ctx, true,
					func(ctx context.Context) (kvtx.Tx, error) {
						return objStore.NewTransaction(ctx, true)
					},
					func(ctx context.Context, tx kvtx.Tx) error {
						return tx.Set(ctx, LinkedCloudKey(sessionID), []byte(t.cloudAccountID))
					},
				)
			}
		}
	}

	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return err
	}

	le.WithField("sess-peer-id", sessionPeerID.String()).Debug("loaded session peer")

	stateAtomMgr := resource_state.NewStateAtomManager(t.a.t.p.b, objectStoreID, volID)
	defer stateAtomMgr.Release()

	so := &Session{
		ctx:                 ctx,
		tkr:                 t,
		objStore:            objStore,
		stateAtomMgr:        stateAtomMgr,
		stateAtomStoreIndex: session.NewStateAtomStoreIndex(objStore),
		sessionPriv:         sessionPriv,
		sessionPid:          sessionPeerID,
		storageKey:          storageKey,
		lockMode:            lockMode,
	}
	so.transitionWatcher = routine.NewRoutineContainerWithLogger(le.WithField("routine", "account-transition"), routine.WithRetry(providerBackoff))
	so.transitionWatcher.SetRoutine(so.waitAccountTransition)

	// An unlocked credential remains available until locking or account shutdown.
	// Pin before publication so temporary mounts cannot discard PIN authorization.
	selfRef, _, _ := t.a.sessions.AddKeyRef(t.id)
	t.setPinnedRef(selfRef.Release)
	defer t.releasePinnedRef()
	defer t.sessionProm.SetPromise(nil)
	t.a.setLinkedCloudAccountID(t.cloudAccountID)

	// Transport and background replication belong to the account. A temporary
	// Session mount ending must not tear down the next consumer's connection.
	// PIN credentials remain authorized only for the unlocked Session lifetime.
	defer func() {
		if so.lockMode == session_lock.SessionLockMode_PIN_ENCRYPTED {
			cleanupCtx, cancel := sessionTransportCleanupContext(ctx)
			defer cancel()
			if err := t.a.stopSessionPeerTransport(cleanupCtx, sessionPeerID); err != nil {
				le.WithError(err).Warn("failed to stop locked session transport")
			}
		}
	}()
	so.transitionWatcher.SetContext(ctx, false)
	defer func() {
		if exited, _ := so.transitionWatcher.SetRoutine(nil); exited != nil {
			<-exited
		}
	}()
	t.sessionProm.SetResult(so, nil)

	// Always start the session transport. Only cloud-linked local accounts
	// need the cloud signaling controller; no-cloud accounts can run direct
	// manual links through the same session bus.
	if err := t.startTransport(ctx, sessionPriv); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// UnlockPINSession unlocks a PIN-locked session before it is mounted.
// Stores the decrypted key on the ProviderAccount for the tracker to consume.
func (a *ProviderAccount) UnlockPINSession(ctx context.Context, ref *session.SessionRef, pin []byte) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	provRef := ref.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	// Build the object store to read PIN lock files.
	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerID, providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.t.p.b, false, objectStoreID, volID, nil)
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

// MountSession attempts to mount a Session returning the session and a release function.
//
// It is usually called by the provider controller.
func (a *ProviderAccount) MountSession(ctx context.Context, ref *session.SessionRef, released func()) (session.Session, func(), error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}

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

	return ws, tkrRef.Release, nil
}

// GetPINSessionRecoveryState reports whether the local PIN reset path has a
// recovery envelope to unlock with a password or backup key.
func (a *ProviderAccount) GetPINSessionRecoveryState(ctx context.Context, ref *session.SessionRef) (session.SessionRecoveryState, error) {
	if err := ref.Validate(); err != nil {
		return session.SessionRecoveryState_SESSION_RECOVERY_STATE_UNKNOWN, err
	}

	provRef := ref.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerID, providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.t.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return session.SessionRecoveryState_SESSION_RECOVERY_STATE_UNKNOWN, errors.Wrap(err, "mount session object store for recovery status")
	}
	defer diRef.Release()

	hasEnvelope, err := HasSessionEnvelope(ctx, objStoreHandle.GetObjectStore(), sessionID)
	if err != nil {
		return session.SessionRecoveryState_SESSION_RECOVERY_STATE_UNKNOWN, err
	}
	if hasEnvelope {
		return session.SessionRecoveryState_SESSION_RECOVERY_STATE_AVAILABLE, nil
	}
	return session.SessionRecoveryState_SESSION_RECOVERY_STATE_UNAVAILABLE, nil
}

// ResetPINSession resets a PIN-locked session via envelope recovery.
// Derives the entity key from the credential, recovers the session
// private key from the envelope, re-encrypts it in auto-unlock mode,
// and restarts the session tracker.
func (a *ProviderAccount) ResetPINSession(ctx context.Context, ref *session.SessionRef, cred *session.EntityCredential) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if cred == nil {
		return errors.New("credential is required for local session reset")
	}

	provRef := ref.GetProviderResourceRef()
	providerID := provRef.GetProviderId()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerID, providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.t.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store for reset")
	}
	defer diRef.Release()

	objStore := objStoreHandle.GetObjectStore()

	// Derive entity private key from credential.
	entityPrivKey, err := resolveEntityPrivKey(providerAccountID, cred)
	if err != nil {
		return err
	}

	// Recover session private key from envelope.
	recoveredKey, err := UnlockSessionFromEnvelope(ctx, objStore, sessionID, entityPrivKey)
	if err != nil {
		return errors.Wrap(err, "envelope recovery")
	}

	// Re-encrypt recovered key in auto-unlock mode.
	pemData, err := keypem.MarshalPrivKeyPem(recoveredKey)
	if err != nil {
		return errors.Wrap(err, "marshal recovered key")
	}
	defer scrub.Scrub(pemData)

	// Derive storage key from volume for auto-unlock encryption.
	volPeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPrivKey, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume private key")
	}
	storageKey, err := session_lock.DeriveStorageKey(volPrivKey)
	if err != nil {
		return errors.Wrap(err, "derive storage key")
	}

	encPriv, err := session_lock.EncryptAutoUnlock(storageKey, pemData)
	if err != nil {
		return errors.Wrap(err, "encrypt recovered key")
	}

	if err := session_lock.WriteAutoUnlock(ctx, objStore, sessionID, encPriv); err != nil {
		return errors.Wrap(err, "write auto-unlock key")
	}

	// Restart the tracker to pick up the recovered key.
	a.sessions.RemoveKey(sessionID)

	return nil
}

// _ is a type assertion.
var (
	_ session.SessionProvider = (*ProviderAccount)(nil)
	_ session.Session         = (*Session)(nil)
)

package provider_local

import (
	"context"
	"crypto/rand"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
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
	return s.sessionPriv
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
	locked := s.sessionPriv == nil
	return session.SessionLockMode(mode), locked, nil
}

// UnlockSession unlocks a PIN-locked session. No-op if already unlocked.
func (s *Session) UnlockSession(ctx context.Context, pin []byte) error {
	// Session is already unlocked if we reached this point.
	return nil
}

// LockSession locks a running session, scrubbing the privkey from memory.
// The session tracker will restart and enter PIN-wait state when re-mounted.
func (s *Session) LockSession(ctx context.Context) error {
	if s.lockMode != session_lock.SessionLockMode_PIN_ENCRYPTED {
		return errors.New("cannot lock: PIN mode not configured")
	}
	if s.sessionPriv == nil {
		return nil
	}

	// Scrub the private key from memory.
	raw, err := s.sessionPriv.Raw()
	if err == nil {
		scrub.Scrub(raw)
	}
	s.sessionPriv = nil

	// Invalidate the session so new mounts trigger a tracker restart.
	s.tkr.sessionProm.SetPromise(nil)
	s.tkr.unlockProm = promise.NewPromiseContainer[[]byte]()

	// Broadcast locked=true so WatchLockState emits.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return nil
}

// SetLockMode changes the session lock mode. Hot switch: session stays running.
func (s *Session) SetLockMode(ctx context.Context, mode session.SessionLockMode, pin []byte) error {
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

// updateSessionMetadata updates the session controller metadata with the current lock mode.
// Reads existing metadata first to avoid clobbering other fields.
func (s *Session) updateSessionMetadata(ctx context.Context, mode session.SessionLockMode) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.GetBus(), "", false, nil)
	if err != nil {
		return
	}
	defer sessionCtrlRef.Release()

	ref := s.GetSessionRef()
	_, meta := session.FindSessionMetadata(ctx, sessionCtrl, ref)
	if meta == nil {
		meta = &session.SessionMetadata{
			ProviderDisplayName: "Local",
			ProviderId:          "local",
		}
	}
	meta.LockMode = mode
	_ = sessionCtrl.UpdateSessionMetadata(ctx, ref, meta)
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
	// a is the provider account
	a *ProviderAccount
	// id is the session id
	id string
	// cloudAccountID links this local session to a cloud account (empty for standalone).
	cloudAccountID string
	// ref is the reference to the session
	// set when instantiating the tracker
	ref *promise.Promise[*session.SessionRef]
	// sessionProm is the session promise container
	sessionProm *promise.PromiseContainer[*Session]
	// unlockProm is set when PIN-locked. Unblocks when UnlockSession is called.
	unlockProm *promise.PromiseContainer[[]byte]
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
		otx, err := objStore.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		data, found, err := otx.Get(ctx, LinkedCloudKey(sessionID))
		otx.Discard()
		if err != nil {
			return err
		}
		if found && len(data) != 0 {
			t.cloudAccountID = string(data)
		}
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
			// Generate new key (first time).
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
				return err
			}

			// Write linked-cloud cross-reference if this session is linked to a cloud account.
			if t.cloudAccountID != "" {
				cotx, cerr := objStore.NewTransaction(ctx, true)
				if cerr == nil {
					defer cotx.Discard()
					_ = cotx.Set(ctx, LinkedCloudKey(sessionID), []byte(t.cloudAccountID))
					_ = cotx.Commit(ctx)
				}
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
	t.sessionProm.SetResult(so, nil)
	defer t.sessionProm.SetPromise(nil)
	t.a.setLinkedCloudAccountID(t.cloudAccountID)

	// Always start the session transport so this peer is reachable on signaling.
	signalingURL := t.a.lookupCloudEndpoint(ctx)
	sts, err := t.a.createSessionTransport(ctx, sessionPriv, signalingURL)
	defer t.a.stopSessionTransportState(sts)
	if err != nil {
		le.WithError(err).Warn("failed to start session transport")
	}
	if st := t.a.GetSessionTransport(); st != nil {
		// Restore P2P sync controllers for accounts that already have paired
		// devices, so a session that was paired in a prior mount resumes
		// DEX/SOSync without requiring an explicit re-pair.
		if err := t.a.AutoStartP2PSyncIfPaired(ctx, st); err != nil {
			le.WithError(err).Warn("failed to auto-start P2P sync on session mount")
		}
	}

	// Wait for context cancel.
	<-ctx.Done()
	return context.Canceled
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
	tkrRef, tkr, _ := a.sessions.AddKeyRef(sessionID)
	tkr.ref.SetResult(ref, nil)
	tkr.unlockProm.SetResult(slices.Clone(privPEM), nil)
	tkrRef.Release()

	return nil
}

// MountSession attempts to mount a Session returning the session and a release function.
//
// usually called by the provider controller
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

// _ is a type assertion
var (
	_ session.SessionProvider = ((*ProviderAccount)(nil))
	_ session.Session         = ((*Session)(nil))
)

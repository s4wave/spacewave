package provider_spacewave

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
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
	pairingMu sync.Mutex
	// pairingEngine serves pairing during the unlocked Session lifetime.
	pairingEngine *pairing.Engine
	// pairingCancel ends the pairing engine, guarded by pairingMu.
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
	meta, err := session.LookupSessionMetadata(ctx, sessionCtrl, ref)
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
	// Serialize unlock with startup and other lock transitions.
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

	// Parse the unlocked key before scrubbing its serialized plaintext.
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
	// Serialize locking with startup and other lock transitions.
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

	// Stop authenticated workers before scrubbing the Session key.
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
	// Serialize mode changes with startup and other lock transitions.
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
	meta, err := session.LookupSessionMetadata(ctx, sessionCtrl, ref)
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
			// Delete the Session lock records before replacing its tracker.
			for _, key := range [][]byte{
				session_lock.MakeKey(sessionID, session_lock.SuffixPK),
				session_lock.MakeKey(sessionID, session_lock.SuffixLocked),
				session_lock.MakeKey(sessionID, session_lock.SuffixLockKey),
				session_lock.MakeKey(sessionID, session_lock.SuffixLockParams),
				session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope),
				[]byte(sessionID + "/registered"),
			} {
				if err := tx.Delete(ctx, key); err != nil {
					return err
				}
			}
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

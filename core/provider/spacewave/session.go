package provider_spacewave

import (
	"context"
	"crypto/rand"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
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
	tkr      *sessionTracker
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
	return s.tkr.a.p.b
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

// UnlockSession unlocks a PIN-locked session with the given PIN.
// No-op if the session is already unlocked.
func (s *Session) UnlockSession(ctx context.Context, pin []byte) error {
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

	privKey, err := keypem.ParsePrivKeyPem(privPEM)
	scrub.Scrub(privPEM)
	if err != nil {
		return errors.Wrap(err, "parse unlocked key")
	}

	s.sessionPriv = privKey
	s.tkr.a.maybeSetSessionClient(s.tkr.id, NewSessionClient(
		s.tkr.a.p.httpCli,
		s.tkr.a.p.endpoint,
		s.tkr.a.p.signingEnvPfx,
		privKey,
		s.sessionPid.String(),
	))
	transportCtx := context.WithoutCancel(ctx)
	if err := s.tkr.a.CreateSessionTransport(transportCtx, privKey, s.tkr.a.p.endpoint); err != nil {
		s.tkr.a.le.WithError(err).Warn("failed to start session transport after unlock")
	}
	if st := s.tkr.a.GetSessionTransport(); st != nil {
		if err := s.tkr.a.StartP2PSync(transportCtx, st); err != nil {
			s.tkr.a.le.WithError(err).Warn("failed to start P2P sync after unlock")
		}
	}

	// Broadcast unlocked state.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return nil
}

// LockSession locks a running session, scrubbing the privkey from memory.
// Existing mounted references remain valid, but future mounts must wait for a
// fresh tracker run instead of reusing this in-memory session.
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

	// Invalidate the published session so future mounts do not reuse it.
	s.tkr.sessionProm.SetPromise(nil)
	s.tkr.unlockProm = promise.NewPromiseContainer[[]byte]()
	s.tkr.releasePinnedRef()
	s.tkr.a.dropSessionClientForSession(s.tkr.id)
	s.tkr.a.StopP2PSync()
	s.tkr.a.StopSessionTransport()

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
	idx, meta := session.FindSessionMetadata(ctx, sessionCtrl, ref)
	if idx == 0 {
		return
	}
	if meta == nil {
		meta = &session.SessionMetadata{
			ProviderDisplayName: "Cloud",
			ProviderId:          "spacewave",
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
	t.pinnedRefMtx.Lock()
	release := t.releasePinnedRefFn
	t.releasePinnedRefFn = nil
	t.pinnedRefMtx.Unlock()
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

	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return err
	}

	le.WithField("sess-peer-id", sessionPeerID.String()).Debug("loaded session peer")

	// Check if this session peer ID is already registered with the cloud.
	regKey := []byte(sessionID + "/registered")
	registered := false
	if err := func() error {
		otx, err := objStore.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer otx.Discard()
		data, found, err := otx.Get(ctx, regKey)
		if err != nil {
			return err
		}
		if found && string(data) == sessionPeerID.String() {
			registered = true
		}
		return nil
	}(); err != nil {
		return errors.Wrap(err, "check session registration state")
	}

	registerSession := func() (*api.ObservedSessionMetadata, error) {
		resp, err := t.a.entityCli.RegisterSessionDirectWithResponse(ctx, sessionPeerID.String(), buildSessionDeviceInfo(), "", "")
		if err != nil {
			return nil, errors.Wrap(err, "register session with cloud")
		}
		le.Debug("registered session with cloud")
		t.a.maybeSetSessionClient(t.id, NewSessionClient(
			t.a.p.httpCli,
			t.a.p.endpoint,
			t.a.p.signingEnvPfx,
			sessionPriv,
			sessionPeerID.String(),
		))
		otx, err := objStore.NewTransaction(ctx, true)
		if err != nil {
			return nil, err
		}
		defer otx.Discard()
		if err := otx.Set(ctx, regKey, []byte(sessionPeerID.String())); err != nil {
			return nil, err
		}
		if err := otx.Commit(ctx); err != nil {
			return nil, err
		}
		return resp.GetObservedMetadata(), nil
	}
	var registeredObserved *api.ObservedSessionMetadata
	if !registered {
		observed, err := registerSession()
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
		sessionPriv:         sessionPriv,
		sessionPid:          sessionPeerID,
		storageKey:          storageKey,
		lockMode:            lockMode,
	}

	// Take a self-ref to keep the tracker alive even when all external
	// refs are released (e.g., between CLI commands).
	// This must happen before publishing sessionProm so callers cannot drop the
	// last external ref in the small window before the tracker pins itself.
	selfRef, _, _ := t.a.sessions.AddKeyRef(t.id)
	t.setPinnedRef(selfRef.Release)
	t.sessionProm.SetResult(so, nil)
	defer t.sessionProm.SetPromise(nil)
	defer t.releasePinnedRef()
	if registeredObserved != nil {
		if err := t.a.UpsertSessionPresentation(ctx, sessionPeerID.String(), registeredObserved); err != nil {
			le.WithError(err).Warn("failed to mirror session presentation metadata")
		}
	}

	// Hold a ref on the account to prevent the account tracker (and its
	// volume) from exiting while this session is alive. Without this,
	// mountNewSession's deferred relProvAcc drops the last account ref,
	// the account tracker exits, the volume dies, and the session's
	// object store becomes invalid.
	accountRef, _, _ := t.a.p.accountRc.AddKeyRef(t.a.accountID)
	defer accountRef.Release()
	defer t.a.StopP2PSync()
	defer t.a.StopSessionTransport()

	if err := t.a.CreateSessionTransport(ctx, sessionPriv, t.a.p.endpoint); err != nil {
		if strings.Contains(err.Error(), "HTTP 401") {
			le.WithError(err).Warn("registered session rejected; re-registering")
			observed, rerr := registerSession()
			if rerr != nil {
				le.WithError(rerr).Warn("failed to re-register rejected session")
			} else if perr := t.a.UpsertSessionPresentation(ctx, sessionPeerID.String(), observed); perr != nil {
				le.WithError(perr).Warn("failed to mirror session presentation metadata")
			} else if terr := t.a.CreateSessionTransport(ctx, sessionPriv, t.a.p.endpoint); terr != nil {
				le.WithError(terr).Warn("failed to start session transport after re-registration")
			} else if st := t.a.GetSessionTransport(); st != nil {
				if perr := t.a.StartP2PSync(ctx, st); perr != nil {
					t.a.le.WithError(perr).Warn("failed to start P2P sync on session mount")
				}
			}
		} else {
			le.WithError(err).Warn("failed to start session transport")
		}
	} else if st := t.a.GetSessionTransport(); st != nil {
		if err := t.a.StartP2PSync(ctx, st); err != nil {
			le.WithError(err).Warn("failed to start P2P sync on session mount")
		}
	}

	<-ctx.Done()
	return context.Canceled
}

// UnlockPINSession unlocks a PIN-locked session before it is mounted.
// Decrypts the session key with the PIN and unblocks the tracker.
func (a *ProviderAccount) UnlockPINSession(ctx context.Context, ref *session.SessionRef, pin []byte) error {
	if err := ref.Validate(); err != nil {
		return err
	}

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
	tkrRef, tkr, _ := a.sessions.AddKeyRef(sessionID)
	tkr.ref.SetResult(ref, nil)
	tkr.unlockProm.SetResult(slices.Clone(privPEM), nil)
	tkrRef.Release()

	return nil
}

// ResetPINSession resets a PIN-locked session by deleting all lock files
// and the stored key. The session tracker will generate a fresh key on
// next mount.
func (a *ProviderAccount) ResetPINSession(ctx context.Context, ref *session.SessionRef, _ *session.EntityCredential) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	provRef := ref.GetProviderResourceRef()
	providerAccountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()

	volID := a.vol.GetID()
	objectStoreID := SessionObjectStoreID(providerAccountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, a.p.b, false, objectStoreID, volID, nil)
	if err != nil {
		return errors.Wrap(err, "mount session object store for reset")
	}
	defer diRef.Release()

	objStore := objStoreHandle.GetObjectStore()

	// Delete all lock files and the stored key.
	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()
	_ = otx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixPK))
	_ = otx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLocked))
	_ = otx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLockKey))
	_ = otx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixLockParams))
	_ = otx.Delete(ctx, session_lock.MakeKey(sessionID, session_lock.SuffixEnvelope))
	// Clear registration so the new key gets re-registered.
	_ = otx.Delete(ctx, []byte(sessionID+"/registered"))
	if err := otx.Commit(ctx); err != nil {
		return errors.Wrap(err, "delete lock files")
	}

	// Restart the tracker so it generates a fresh key.
	a.sessions.RemoveKey(sessionID)

	return nil
}

// MountSession attempts to mount a Session returning the session and a release function.
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

// _ is a type assertion
var (
	_ session.SessionProvider = ((*ProviderAccount)(nil))
	_ session.Session         = ((*Session)(nil))
)

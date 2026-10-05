package provider_spacewave

import (
	"context"
	"crypto/rand"
	"sync"

	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
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

	// Bind this tracker to logout before it acquires credentials or starts workers.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, t.a.p.b, "", false, nil)
	if err != nil {
		return errors.Wrap(err, "lookup session lifetime controller")
	}
	defer sessionCtrlRef.Release()
	finished := promise.NewPromise[struct{}]()
	releaseLifetime := sessionCtrl.TrackSession(sessionRef, func(stopCtx context.Context) error {
		t.a.sessions.RemoveKey(t.id)
		_, err := finished.Await(stopCtx)
		t.sessionProm.SetResult(nil, session.ErrSessionNotFound)
		return err
	})
	defer func() {
		finished.SetResult(struct{}{}, nil)
		releaseLifetime()
	}()

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
	sessionPriv, err := t.loadSessionPrivateKey(ctx, objStore, lockMode, storageKey)
	if err != nil {
		return err
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

	// Release the account signer even when registration or later startup fails.
	defer t.a.dropSessionClientForSession(t.id)

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
	sessionMeta, err := session.LookupSessionMetadata(ctx, sessionCtrl, sessionRef)
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

	// Prepare the account transition and transport authorization watchers.
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

	// Watch account transitions until the Session stops.
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

	// Let callers waiting on startup proceed.
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

// loadSessionPrivateKey unlocks the persisted Session key or stores a fresh auto-unlock key.
func (t *sessionTracker) loadSessionPrivateKey(ctx context.Context, objStore object.ObjectStore, mode session_lock.SessionLockMode, storageKey [32]byte) (crypto.PrivKey, error) {
	// Wait for the PIN unlock operation to supply its decrypted key.
	if mode == session_lock.SessionLockMode_PIN_ENCRYPTED {
		privPEM, err := t.unlockProm.Await(ctx)
		if err != nil {
			return nil, err
		}
		defer scrub.Scrub(privPEM)
		return keypem.ParsePrivKeyPem(privPEM)
	}

	// Read the auto-unlock key from the account's Session store.
	data, found, err := session_lock.ReadAutoUnlockKey(ctx, objStore, t.id)
	if err != nil {
		return nil, errors.Wrap(err, "read auto-unlock key")
	}

	// Decrypt an existing auto-unlock key and release its plaintext after parsing.
	if found {
		privPEM, err := session_lock.DecryptAutoUnlock(storageKey, data)
		if err != nil {
			return nil, errors.Wrap(err, "decrypt auto-unlock key")
		}
		defer scrub.Scrub(privPEM)
		return keypem.ParsePrivKeyPem(privPEM)
	}

	// Create the first key for this Session and serialize it for encrypted storage.
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	privPEM, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(privPEM)

	// Persist the encrypted key before publishing the Session's signer.
	encPriv, err := session_lock.EncryptAutoUnlock(storageKey, privPEM)
	if err != nil {
		return nil, err
	}
	if err := session_lock.WriteAutoUnlock(ctx, objStore, t.id, encPriv); err != nil {
		return nil, errors.Wrap(err, "write auto-unlock key")
	}
	return priv, nil
}

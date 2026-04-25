package provider_spacewave

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// providerAccountTracker tracks a ProviderAccount in the world.
type providerAccountTracker struct {
	// p is the provider
	p *Provider
	// accountID is the account identifier
	accountID string
	// accCtr is the provider account container
	accCtr *ccontainer.CContainer[*ProviderAccount]
}

// ProviderAccount implements the spacewave provider account.
type ProviderAccount struct {
	// t is the tracker
	t *providerAccountTracker
	// le is the logger
	le *logrus.Entry
	// p is the provider
	p *Provider
	// accountID is the account identifier on the cloud
	accountID string
	// vol is the parent volume for storage for the account
	vol volume.Volume
	// entityCli is the entity client for registration flows
	entityCli *EntityClient
	// sessionClient is the session client for authenticated API
	sessionClient *SessionClient
	// sessionClientSessionID is the mounted session that owns sessionClient.
	sessionClientSessionID string
	// sessionTransport is the running session-scoped transport for direct P2P.
	sessionTransport *sessionTransportState
	// conf is the provider configuration
	conf *Config
	// sfs is the step factory set for block transforms
	sfs *block_transform.StepFactorySet
	// objStore is the ObjectStore for account-level persistence (state cache).
	objStore object.ObjectStore

	// wsTracker is the shared session WebSocket tracker.
	wsTracker *wsTracker
	// soListCtr is the persistent shared object list container.
	soListCtr *ccontainer.CContainer[*sobject.SharedObjectList]
	// soListRc owns SO list fetches so all callers share one invalidatable path.
	soListRc *refcount.RefCount[struct{}]
	// soListBcast guards the SO list owner fields below.
	soListBcast broadcast.Broadcast
	// soListAccess indicates subscription status permits SO list access.
	soListAccess bool
	// soListInvalidate restarts the SO list owner when the cache is stale.
	soListInvalidate func()
	// writeTicketOwnersMtx guards writeTicketOwners and writeTicketOwnersCtx.
	writeTicketOwnersMtx sync.Mutex
	// writeTicketOwners caches per-resource bundled write-ticket owners.
	writeTicketOwners map[string]*writeTicketOwner
	// writeTicketOwnersCtx is the lifecycle context shared by ticket owners.
	writeTicketOwnersCtx context.Context
	// selfRejoinSweep opportunistically heals missing same-entity SO peers after
	// a new session registers or reconnect invalidates sweep-side caches.
	selfRejoinSweep *routine.StateRoutineContainer[*selfRejoinSweepState]
	// sessionPresentationReconcile prunes orphaned mirrored session metadata.
	sessionPresentationReconcile *routine.StateRoutineContainer[*sessionPresentationReconcileState]
	// accountFetcherRoutine owns account-state refetches for this account.
	accountFetcherRoutine *routine.RoutineContainer
	// orgProcessors watches org SO membership and runs org processors.
	orgProcessors *routine.RoutineContainer
	// entityKeyStore holds unlocked entity keypairs shared across account
	// resources for this provider account.
	entityKeyStore *EntityKeyStore
	// entityKeypairStepUpRc retains unlocked entity keypairs until the last
	// screen-scoped step-up reference is released.
	entityKeypairStepUpRc *refcount.RefCount[struct{}]
	// managedBAsRc caches the billing accounts created by this caller.
	managedBAsRc *refcount.RefCount[*managedBAsSnapshot]
	// p2pSync contains the direct invite / sync controllers bound to the
	// current session transport.
	p2pSync *p2pSyncState

	// bstores contains the set of mounted block stores.
	bstores *keyed.KeyedRefCount[string, *bstoreTracker]
	// sobjects contains the set of mounted shared objects.
	sobjects *keyed.KeyedRefCount[string, *sobjectTracker]
	// sessions contains the set of mounted sessions.
	sessions *keyed.KeyedRefCount[string, *sessionTracker]

	// checkoutWatcher manages the checkout status WebSocket.
	checkoutWatcher *checkoutWatcher

	// accountBcast fires when account state changes.
	// Guards all fields in the accountState struct below.
	accountBcast broadcast.Broadcast
	// transportBcast guards sessionTransport.
	transportBcast broadcast.Broadcast
	// syncTelemetryBcast guards syncTelemetry.
	syncTelemetryBcast broadcast.Broadcast
	// syncTelemetry stores sync activity snapshots keyed by block store id.
	syncTelemetry map[string]*syncTelemetryState

	// orgBcast fires when org list changes.
	// Guards orgList, orgListValid, and orgSnapshotRcs.
	orgBcast broadcast.Broadcast
	// orgList is the cached org list from the cloud.
	orgList []*api.OrgResponse
	// orgListValid indicates orgList has been fetched at least once.
	orgListValid bool
	// orgSnapshotRcs caches full organization detail snapshots keyed by org id.
	orgSnapshotRcs map[string]*refcount.RefCount[*organizationSnapshot]
	// orgSyncs serializes org refresh and reconciliation work keyed by org id.
	orgSyncs *keyed.Keyed[string, struct{}]
	// pendingParticipantSyncs serializes pending participant reconciliation work.
	pendingParticipantSyncs *keyed.Keyed[pendingParticipantSyncKey, struct{}]
	// memberSessionSyncs serializes member session add/remove reconciliation work.
	memberSessionSyncs *keyed.Keyed[memberSessionSyncKey, struct{}]
	// mailboxAutoEntriesMtx guards mailboxAutoEntries.
	mailboxAutoEntriesMtx sync.Mutex
	// mailboxAutoEntries stores pending owner-side mailbox auto-process payloads.
	mailboxAutoEntries map[mailboxAutoProcessKey]*api.MailboxEntry
	// mailboxAutoProcessors serializes owner-side mailbox auto-processing.
	mailboxAutoProcessors *keyed.Keyed[mailboxAutoProcessKey, struct{}]
	// cdnRootChangedMtx guards cdnRootChangedCbs.
	cdnRootChangedMtx sync.Mutex
	// cdnRootChangedCbs fans out cdn-root-changed session WS frames to
	// subscribers. Session resources that mount the anonymous CDN Space
	// register here so they can invalidate their cached root pointer when
	// the cloud publishes a new =root.packedmsg=.
	cdnRootChangedCbs map[*func(spaceID string)]struct{}
	// state holds all cached account state guarded by accountBcast.
	state accountState
}

// RegisterCdnRootChangedCallback subscribes cb to cdn-root-changed session
// WebSocket frames. Returns a release function that unsubscribes; callers
// MUST release when the subscription is no longer needed. cb is invoked
// from the WS read loop so it should be non-blocking or offload work.
func (acc *ProviderAccount) RegisterCdnRootChangedCallback(cb func(spaceID string)) func() {
	if cb == nil {
		return func() {}
	}
	key := &cb
	acc.cdnRootChangedMtx.Lock()
	if acc.cdnRootChangedCbs == nil {
		acc.cdnRootChangedCbs = make(map[*func(spaceID string)]struct{})
	}
	acc.cdnRootChangedCbs[key] = struct{}{}
	acc.cdnRootChangedMtx.Unlock()
	return func() {
		acc.cdnRootChangedMtx.Lock()
		delete(acc.cdnRootChangedCbs, key)
		acc.cdnRootChangedMtx.Unlock()
	}
}

// fireCdnRootChanged fans out a cdn-root-changed notification to every
// registered callback. Callbacks are invoked under a snapshot so Register
// and Release calls do not block the WS reader.
func (acc *ProviderAccount) fireCdnRootChanged(spaceID string) {
	var cbs []func(string)
	acc.cdnRootChangedMtx.Lock()
	for key := range acc.cdnRootChangedCbs {
		cbs = append(cbs, *key)
	}
	acc.cdnRootChangedMtx.Unlock()
	for _, cb := range cbs {
		cb(spaceID)
	}
}

// accountState holds all cached account state for a provider account.
// All fields are guarded by ProviderAccount.accountBcast.
type accountState struct {
	// info is the cached account state from GET /account/state.
	info *api.AccountStateResponse
	// infoFetching indicates a GET /account/state fetch is in flight.
	infoFetching bool
	// sessions is the cached cloud auth session set from GET /account/sessions.
	sessions []*api.AccountSessionInfo
	// sessionsValid indicates sessions has been populated at least once.
	sessionsValid bool
	// epoch is the current known epoch (may be ahead of info.Epoch).
	epoch uint64
	// lastFetchedEpoch is the epoch at the time of the last successful fetch.
	lastFetchedEpoch uint64
	// accountBootstrapFetched indicates a live account fetch completed in this
	// provider-account lifecycle. Session registration defers the eager
	// self-rejoin sweep until this flips true so startup does not schedule the
	// same sweep both before and after the initial account bootstrap fetch.
	accountBootstrapFetched bool
	// selfRejoinSweepGeneration increments when a new local session registers or
	// a reconnect invalidates the caches the sweep depends on.
	selfRejoinSweepGeneration uint64
	// selfRejoinSweepRunning indicates the automatic session rejoin sweep is
	// processing readable shared objects.
	selfRejoinSweepRunning bool
	// selfEnrollmentSummary is the cached root-route self-enrollment predicate.
	selfEnrollmentSummary *SelfEnrollmentSummary
	// selfEnrollmentSkippedGenerationKey is the backend skip generation key.
	selfEnrollmentSkippedGenerationKey string
	// status is the current account status (READY, UNAUTHENTICATED, DELETED).
	status provider.ProviderAccountStatus

	// cachedEmails is the cached email list from GET /account/emails.
	// Refreshed alongside account state when epoch changes.
	cachedEmails []*api.AccountEmailInfo
	// cachedEmailsValid indicates cachedEmails has been populated at least once.
	cachedEmailsValid bool
	// billingSnapshotRcs caches billing account state and usage by billing account id.
	billingSnapshotRcs map[string]*refcount.RefCount[*billingSnapshot]
	// sharedObjectMetadata caches full shared-object metadata by SO ID.
	sharedObjectMetadata map[string]*sharedObjectMetadataState
	// pendingMailboxEntries caches owner-visible pending mailbox metadata by SO ID.
	pendingMailboxEntries map[string]*pendingMailboxState
	// mailboxRequestStatus tracks invitee-visible mailbox status updates by SO,
	// invite, and peer for cloud invite joins.
	mailboxRequestStatus map[mailboxRequestKey]string
}

// buildProviderAccountTracker builds a new providerAccountTracker for an account id.
func (p *Provider) buildProviderAccountTracker(accountID string) (keyed.Routine, *providerAccountTracker) {
	accCtr := ccontainer.NewCContainer[*ProviderAccount](nil)
	tracker := &providerAccountTracker{
		p:         p,
		accountID: accountID,
		accCtr:    accCtr,
	}
	return tracker.executeProviderAccountTracker, tracker
}

// executeProviderAccountTracker executes the provider account tracker routine.
func (t *providerAccountTracker) executeProviderAccountTracker(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	le := t.p.le.WithField("account-id", t.accountID)

	// Mount the storage volume for this account.
	storageVolumeID := StorageVolumeID(t.accountID)
	volumeID := storageVolumeID
	volCtrl, _, volCtrlRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		t.p.b,
		resolver.NewLoadControllerWithConfig(&storage_volume.Config{
			StorageId:       "default",
			StorageVolumeId: storageVolumeID,
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{volumeID},
			},
		}),
		ctxCancel,
	)
	if err != nil {
		return err
	}
	defer volCtrlRef.Release()

	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Mount ObjectStore for account-level persistence (state cache).
	objStoreID := AccountStateCacheID(t.accountID)
	objStoreHandle, _, objDiRef, err := volume.ExBuildObjectStoreAPI(
		ctx, t.p.b, false, objStoreID, vol.GetID(), ctxCancel,
	)
	if err != nil {
		return errors.Wrap(err, "mounting account object store")
	}
	defer objDiRef.Release()
	objStore := objStoreHandle.GetObjectStore()

	// Build entity client for registration flows.
	// Use cached entity key if available (set during account creation).
	var entityCli *EntityClient
	entityKeyStore := t.p.GetEntityKeyStore(t.accountID)
	if priv, pid, ok := entityKeyStore.GetAnyUnlockedKey(); ok {
		entityCli = NewEntityClientDirect(
			t.p.httpCli,
			t.p.endpoint,
			t.p.signingEnvPfx,
			priv,
			pid,
		)
	} else {
		entityCli = NewEntityClient(
			t.p.httpCli,
			t.p.endpoint,
			t.p.signingEnvPfx,
			t.p.peer,
		)
	}

	// Build session client (key set later by session tracker).
	sessionCli := NewSessionClient(
		t.p.httpCli,
		t.p.endpoint,
		t.p.signingEnvPfx,
		nil,
		"",
	)

	acc := &ProviderAccount{
		t:              t,
		le:             le,
		p:              t.p,
		accountID:      t.accountID,
		vol:            vol,
		objStore:       objStore,
		entityCli:      entityCli,
		sessionClient:  sessionCli,
		conf:           t.p.conf,
		sfs:            t.p.sfs,
		soListCtr:      t.p.getSOListCtr(t.accountID),
		entityKeyStore: entityKeyStore,
	}
	acc.soListCtr.SetValue(nil)
	acc.soListRc = refcount.NewRefCount(nil, true, nil, nil, acc.resolveSharedObjectList)
	acc.entityKeypairStepUpRc = refcount.NewRefCount(
		ctx,
		false,
		nil,
		nil,
		acc.resolveEntityKeypairStepUp,
	)
	acc.managedBAsRc = refcount.NewRefCountWithOptions(
		context.Background(),
		true,
		nil,
		nil,
		acc.resolveManagedBAs,
		snapshotRefCountOptions,
	)
	acc.selfRejoinSweep = routine.NewStateRoutineContainerWithLogger(
		equalSelfRejoinSweepState,
		le.WithField("component", "self-rejoin-sweep"),
		routine.WithRetry(providerBackoff),
	)
	acc.selfRejoinSweep.SetStateRoutine(acc.runSelfRejoinSweep)
	acc.primeSelfRejoinSweepFromUnlockedEntityKeys()
	acc.sessionPresentationReconcile = routine.NewStateRoutineContainerWithLogger(
		equalSessionPresentationReconcileState,
		le.WithField("component", "session-presentation-reconcile"),
		routine.WithRetry(providerBackoff),
	)
	acc.sessionPresentationReconcile.SetStateRoutine(acc.runSessionPresentationReconcile)
	acc.accountFetcherRoutine = routine.NewRoutineContainerWithLogger(
		le.WithField("component", "account-fetcher"),
		routine.WithExitCb(func(err error) {
			if err != nil && !errors.Is(err, context.Canceled) {
				le.WithError(err).Warn("account fetcher exited")
			}
		}),
	)
	acc.accountFetcherRoutine.SetRoutine(acc.accountFetcher)
	acc.orgProcessors = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "org-processors"),
		routine.WithRetry(providerBackoff),
	)
	acc.orgProcessors.SetRoutine(acc.watchOrgProcessors)

	acc.checkoutWatcher = newCheckoutWatcher(
		le.WithField("component", "checkout-watcher"),
		func() *SessionClient { return acc.sessionClient },
		func() {
			// Checkout completed: bump epoch to trigger a fresh fetch of the
			// updated subscription status from the cloud.
			acc.BumpLocalEpoch()
		},
	)

	acc.wsTracker = newWSTracker(
		le.WithField("component", "session-tracker"),
		func() *SessionClient { return acc.sessionClient },
	)
	acc.wsTracker.accountBcast = &acc.accountBcast
	acc.wsTracker.onAccountChanged = func(epoch uint64) {
		le.WithField("epoch", epoch).Debug("account changed via ws notify")
		if epoch > 0 {
			acc.setEpoch(epoch)
		} else {
			acc.BumpLocalEpoch()
		}
	}
	acc.wsTracker.onOrgChanged = func(orgID string) {
		le.WithField("org-id", orgID).Debug("org changed via ws notify")
		acc.InvalidateBillingSnapshot("")
		acc.orgSyncs.SetKey(orgID, true)
	}
	acc.wsTracker.onPendingParticipant = func(soID, accountID string) {
		le.WithField("sobject-id", soID).
			WithField("target-account-id", accountID).
			Debug("pending participant via ws notify")
		acc.pendingParticipantSyncs.SetKey(pendingParticipantSyncKey{
			soID:      soID,
			accountID: accountID,
		}, true)
	}
	acc.wsTracker.onMemberSessionChanged = func(soID, sessionPeerID, accountID string, added bool) {
		le.WithField("sobject-id", soID).
			WithField("session-peer-id", sessionPeerID).
			WithField("account-id", accountID).
			WithField("added", added).
			Debug("member session changed via ws notify")
		acc.memberSessionSyncs.SetKey(memberSessionSyncKey{
			soID:          soID,
			sessionPeerID: sessionPeerID,
			accountID:     accountID,
			added:         added,
		}, true)
	}
	acc.wsTracker.onSONotify = func(soID string, payload *api.SONotifyEventPayload) {
		acc.handleAccountSONotify(soID, payload)
	}
	acc.wsTracker.onSOListUpdate = func(list *sobject.SharedObjectList) {
		acc.soListCtr.SetValue(list)
		acc.refreshSelfEnrollmentSummary(ctx)
	}
	acc.wsTracker.onReconnected = func() {
		le.Debug("session ws reconnected; invalidating event-driven caches")
		acc.InvalidatePendingMailboxEntries()
		acc.InvalidateSharedObjectMetadataCache()
		acc.invalidateSharedObjectList()
		// Re-evaluate every mounted SO once on reconnect: the cold-start gate
		// short-circuits warm SOs and the cache-aware classifier fetches only
		// what is missing on the rest, so a long disconnect window cannot leave
		// caches stale across all SOs without producing per-mount rejoin storms.
		acc.bumpSelfRejoinSweepGeneration()
	}
	acc.wsTracker.onInviteMailbox = func(soID string, entry *api.MailboxEntry, updatedAt int64) {
		if soID == "" || entry == nil {
			return
		}
		le.WithField("sobject-id", soID).
			WithField("entry-id", entry.GetId()).
			Debug("received mailbox add via ws notify")
		acc.ApplyMailboxEntryEvent(soID, entry, updatedAt)
		acc.triggerMailboxEntryAutoProcess(ctx, soID, entry)
	}
	acc.wsTracker.onInviteMailboxUpdate = func(soID string, entry *api.MailboxEntry, updatedAt int64) {
		if soID == "" || entry == nil {
			return
		}
		le.WithField("sobject-id", soID).
			WithField("entry-id", entry.GetId()).
			WithField("status", entry.GetStatus()).
			Debug("received mailbox update via ws notify")
		acc.ApplyMailboxEntryEvent(soID, entry, updatedAt)
		if entry.GetStatus() == "accepted" {
			acc.invalidateSharedObjectList()
		}
		acc.triggerMailboxEntryAutoProcess(ctx, soID, entry)
	}
	acc.wsTracker.onUpdateAvailable = func() {
		le.Debug("dispatching launcher recheck after update_available notify")
		go func() {
			if err := spacewave_launcher.ExRecheckDistConfig(ctx, t.p.b, ""); err != nil && !errors.Is(err, context.Canceled) {
				le.WithError(err).Warn("launcher recheck failed")
			}
		}()
	}
	acc.wsTracker.onCdnRootChanged = func(spaceID string) {
		le.WithField("space-id", spaceID).
			Debug("dispatching cdn-root-changed invalidate to subscribers")
		acc.fireCdnRootChanged(spaceID)
	}
	acc.wsTracker.onDormantChanged = func(dormant bool) {
		acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if dormant {
				acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT
			} else {
				acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
			}
			broadcast()
		})
		acc.refreshSelfRejoinSweepState()
		if dormant {
			acc.syncSharedObjectListAccess(
				s4wave_provider_spacewave.BillingStatus_BillingStatus_UNKNOWN,
			)
			return
		}
		// Exiting dormant means cloud access reopened; force a fresh account
		// state fetch so onboarding reflects the restored subscription.
		acc.BumpLocalEpoch()
	}
	acc.wsTracker.onSessionUnauthenticated = func() {
		acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			acc.state.status = unauthenticatedAccountStatus(acc.state.info)
			broadcast()
		})
		acc.refreshSelfRejoinSweepState()
	}
	acc.wsTracker.onAccountWasDeleted = func() {
		// Remove GC root edge for this provider, cascading all buckets and blocks.
		if kvVol, ok := acc.vol.(kvtx_volume.KvtxVolume); ok {
			if rg := kvVol.GetRefGraph(); rg != nil {
				providerID := acc.p.info.GetProviderId()
				gcOps := block_gc.NewGCStoreOps(acc.vol, rg)
				if err := gcOps.RemoveGCRef(ctx,
					block_gc.NodeGCRoot,
					ProviderIRI(providerID),
				); err != nil {
					le.WithError(err).Warn("GC: failed to remove provider edge")
				}
				if stats, err := block_gc.NewCollector(rg, acc.vol, nil).
					Collect(ctx); err != nil {
					le.WithError(err).Warn("GC: collect on account delete failed")
				} else if stats != nil && stats.NodesSwept > 0 {
					le.Infof("GC swept %d nodes on account deletion", stats.NodesSwept)
				}
			}
		}

		// Set account status to DELETED and broadcast so Watch loops see it.
		acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_DELETED
			broadcast()
		})
		acc.refreshSelfRejoinSweepState()
		acc.p.accountRc.RestartRoutine(acc.accountID)
	}

	acc.bstores = keyed.NewKeyedRefCountWithLogger(
		acc.buildBlockStoreTracker,
		le,
		keyed.WithRetry[string, *bstoreTracker](providerBackoff),
	)
	var orgSyncs *keyed.Keyed[string, struct{}]
	orgSyncs = keyed.NewKeyedWithLogger(
		acc.buildOrgSyncRoutine,
		le.WithField("subsystem", "org-sync"),
		keyed.WithRetry[string, struct{}](providerBackoff),
		keyed.WithExitCb(func(key string, _ keyed.Routine, _ struct{}, err error) {
			if err == nil {
				orgSyncs.RemoveKey(key)
			}
		}),
	)
	acc.orgSyncs = orgSyncs
	var pendingParticipantSyncs *keyed.Keyed[pendingParticipantSyncKey, struct{}]
	pendingParticipantSyncs = keyed.NewKeyedWithLogger(
		acc.buildPendingParticipantSyncRoutine,
		le.WithField("subsystem", "pending-participants"),
		keyed.WithRetry[pendingParticipantSyncKey, struct{}](providerBackoff),
		keyed.WithExitCb(func(key pendingParticipantSyncKey, _ keyed.Routine, _ struct{}, err error) {
			if err == nil {
				pendingParticipantSyncs.RemoveKey(key)
			}
		}),
	)
	acc.pendingParticipantSyncs = pendingParticipantSyncs
	var memberSessionSyncs *keyed.Keyed[memberSessionSyncKey, struct{}]
	memberSessionSyncs = keyed.NewKeyedWithLogger(
		acc.buildMemberSessionSyncRoutine,
		le.WithField("subsystem", "member-sessions"),
		keyed.WithRetry[memberSessionSyncKey, struct{}](providerBackoff),
		keyed.WithExitCb(func(key memberSessionSyncKey, _ keyed.Routine, _ struct{}, err error) {
			if err == nil {
				memberSessionSyncs.RemoveKey(key)
			}
		}),
	)
	acc.memberSessionSyncs = memberSessionSyncs
	acc.sobjects = keyed.NewKeyedRefCountWithLogger(
		acc.buildSharedObjectTracker,
		le,
		keyed.WithRetry[string, *sobjectTracker](providerBackoff),
	)
	var mailboxAutoProcessors *keyed.Keyed[mailboxAutoProcessKey, struct{}]
	mailboxAutoProcessors = keyed.NewKeyedWithLogger(
		acc.buildMailboxAutoProcessRoutine,
		le.WithField("subsystem", "mailbox-auto-process"),
		keyed.WithRetry[mailboxAutoProcessKey, struct{}](providerBackoff),
		keyed.WithExitCb(func(key mailboxAutoProcessKey, _ keyed.Routine, _ struct{}, err error) {
			if err == nil {
				acc.clearMailboxAutoProcessEntry(key)
				mailboxAutoProcessors.RemoveKey(key)
			}
		}),
	)
	acc.mailboxAutoProcessors = mailboxAutoProcessors
	acc.sessions = keyed.NewKeyedRefCountWithLogger(
		acc.buildSessionTracker,
		le,
		keyed.WithRetry[string, *sessionTracker](providerBackoff),
	)

	// Start keyed managers.
	acc.bstores.SetContext(ctx, true)
	defer acc.bstores.ClearContext()

	acc.orgSyncs.SetContext(ctx, true)
	defer acc.orgSyncs.ClearContext()

	acc.pendingParticipantSyncs.SetContext(ctx, true)
	defer acc.pendingParticipantSyncs.ClearContext()

	acc.memberSessionSyncs.SetContext(ctx, true)
	defer acc.memberSessionSyncs.ClearContext()

	acc.sobjects.SetContext(ctx, true)
	defer acc.sobjects.ClearContext()

	acc.mailboxAutoProcessors.SetContext(ctx, true)
	defer acc.mailboxAutoProcessors.ClearContext()

	acc.sessions.SetContext(ctx, true)
	defer acc.sessions.ClearContext()

	acc.checkoutWatcher.SetContext(ctx)
	defer acc.checkoutWatcher.ClearContext()

	acc.wsTracker.SetContext(ctx)
	defer acc.wsTracker.ClearContext()
	wsTrackerRef := acc.wsTracker.AddRef()
	defer wsTrackerRef.Release()

	_ = acc.soListRc.SetContext(ctx)
	defer acc.soListRc.ClearContext()

	acc.setWriteTicketOwnersContext(ctx)
	defer acc.setWriteTicketOwnersContext(nil)

	acc.selfRejoinSweep.SetContext(ctx, true)
	defer acc.selfRejoinSweep.ClearContext()

	acc.sessionPresentationReconcile.SetContext(ctx, true)
	defer acc.sessionPresentationReconcile.ClearContext()

	acc.accountFetcherRoutine.SetContext(ctx, false)
	defer acc.accountFetcherRoutine.ClearContext()

	acc.orgProcessors.SetContext(ctx, true)
	defer acc.orgProcessors.ClearContext()

	// Load cached account state from ObjectStore for instant bootstrap.
	if cached, err := acc.loadAccountStateCache(ctx); err != nil {
		le.WithError(err).Warn("failed to load account state cache")
	} else if cached != nil && cached.GetState() != nil {
		state := cached.GetState()
		var reconcileState *sessionPresentationReconcileState
		acc.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			acc.state.info = state
			acc.state.status = loadedAccountStatus(state)
			acc.state.lastFetchedEpoch = uint64(cached.GetFetchedEpoch())
			reconcileState = acc.buildSessionPresentationReconcileStateLocked()
			broadcast()
		})
		acc.setSessionPresentationReconcileState(reconcileState)
		le.WithField("epoch", cached.GetFetchedEpoch()).Debug("loaded cached account state")
		acc.syncSharedObjectListAccess(state.GetSubscriptionStatus())
	}

	// Bump epoch to trigger the initial account state fetch.
	acc.BumpLocalEpoch()

	// Startup complete.
	t.accCtr.SetValue(acc)
	defer t.accCtr.SetValue(nil)

	<-ctx.Done()
	return context.Canceled
}

// GetSessionClient returns the session client for authenticated API calls.
func (a *ProviderAccount) GetSessionClient() *SessionClient {
	var cli *SessionClient
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cli = a.sessionClient
	})
	if cli == nil || cli.priv == nil || cli.peerID == "" {
		return nil
	}
	return a.configureSessionClient(cli)
}

// ReplaceSessionClient replaces the session client with a new one.
// Used during reauthentication to install a freshly-generated session key.
func (a *ProviderAccount) ReplaceSessionClient(cli *SessionClient) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.sessionClient = a.configureSessionClient(cli)
		a.sessionClientSessionID = ""
		broadcast()
	})
	a.refreshSelfEnrollmentSummary(context.Background())
}

// ReplaceEntityClient replaces the entity client used for registration flows.
func (a *ProviderAccount) ReplaceEntityClient(cli *EntityClient) {
	if cli == nil {
		return
	}
	a.entityCli = cli
}

// GetEntityClient returns the entity client for registration flows.
func (a *ProviderAccount) GetEntityClient() *EntityClient {
	return a.entityCli
}

// GetVolume returns the parent volume for the account.
func (a *ProviderAccount) GetVolume() volume.Volume {
	return a.vol
}

// GetStepFactorySet returns the block transform step factory set.
func (a *ProviderAccount) GetStepFactorySet() *block_transform.StepFactorySet {
	return a.sfs
}

// GetAccountID returns the account identifier.
func (a *ProviderAccount) GetAccountID() string {
	return a.accountID
}

// GetProviderID returns the provider identifier.
func (a *ProviderAccount) GetProviderID() string {
	return a.p.info.GetProviderId()
}

// GetProvider returns the parent Provider.
func (a *ProviderAccount) GetProvider() *Provider {
	return a.p
}

// GetCheckoutWatcher returns the checkout status watcher.
func (a *ProviderAccount) GetCheckoutWatcher() *checkoutWatcher {
	return a.checkoutWatcher
}

// GetLogger returns the logger for this account.
func (a *ProviderAccount) GetLogger() *logrus.Entry {
	return a.le
}

// GetAccountBroadcast returns the broadcast that fires on account state changes.
func (a *ProviderAccount) GetAccountBroadcast() *broadcast.Broadcast {
	return &a.accountBcast
}

// GetOrgBroadcast returns the broadcast that fires on org cache changes.
func (a *ProviderAccount) GetOrgBroadcast() *broadcast.Broadcast {
	return &a.orgBcast
}

// GetCachedOrganization returns the cached org summary when available.
func (a *ProviderAccount) GetCachedOrganization(orgID string) *api.OrgResponse {
	if orgID == "" {
		return nil
	}

	var org *api.OrgResponse
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.orgListValid {
			return
		}
		for _, candidate := range a.orgList {
			if candidate.GetId() != orgID {
				continue
			}
			org = candidate.CloneVT()
			return
		}
	})
	return org
}

// GetAccountStatus returns the current account status.
// Must be called within an accountBcast HoldLock or the result may be stale.
func (a *ProviderAccount) GetAccountStatus() provider.ProviderAccountStatus {
	return a.state.status
}

// SetAccountStatus sets the account status and broadcasts the change.
func (a *ProviderAccount) SetAccountStatus(status provider.ProviderAccountStatus) {
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.state.status = status
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})
	a.setSelfRejoinSweepState(rejoinState)
}

// setEpoch sets the epoch to max(current, n) and broadcasts if changed.
// Called by wsTracker when an account_changed event arrives with an epoch.
func (a *ProviderAccount) setEpoch(n uint64) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if n > a.state.epoch {
			a.state.epoch = n
			for _, rc := range a.state.billingSnapshotRcs {
				rc.Invalidate()
			}
			a.getManagedBAsRcLocked().Invalidate()
			broadcast()
		}
	})
}

// BumpLocalEpoch increments the local epoch by 1 and broadcasts.
// Called by RPC handlers after a successful mutation to trigger a fetch.
func (a *ProviderAccount) BumpLocalEpoch() {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.state.epoch++
		for _, rc := range a.state.billingSnapshotRcs {
			rc.Invalidate()
		}
		a.getManagedBAsRcLocked().Invalidate()
		broadcast()
	})
}

// SetCachedPrimaryEmail updates the cached email snapshot immediately after a
// successful primary-email mutation so WatchEmails subscribers see the change
// without waiting for the next cloud fetch cycle.
func (a *ProviderAccount) SetCachedPrimaryEmail(email string) {
	if email == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !a.state.cachedEmailsValid || len(a.state.cachedEmails) == 0 {
			return
		}

		next := make([]*api.AccountEmailInfo, len(a.state.cachedEmails))
		var changed bool
		var found bool
		for i, row := range a.state.cachedEmails {
			if row == nil {
				continue
			}
			clone := row.CloneVT()
			primary := clone.GetEmail() == email
			if primary {
				found = true
			}
			if clone.GetPrimary() != primary {
				clone.Primary = primary
				changed = true
			}
			next[i] = clone
		}
		if !found || !changed {
			return
		}
		a.state.cachedEmails = next
		broadcast()
	})
}

// KeypairsSnapshot returns the cached keypairs slice.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) KeypairsSnapshot() []*session.EntityKeypair {
	if a.state.info == nil {
		return nil
	}
	return a.state.info.GetKeypairs()
}

// AuthMethodsSnapshot returns the cached auth-method rows.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) AuthMethodsSnapshot() []*api.AccountAuthMethod {
	if a.state.info == nil {
		return nil
	}
	return a.state.info.GetAuthMethods()
}

// AccountStateSnapshot returns the full cached account state.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) AccountStateSnapshot() *api.AccountStateResponse {
	return a.state.info
}

// SessionsSnapshot returns the cached cloud auth session set.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) SessionsSnapshot() ([]*api.AccountSessionInfo, bool) {
	return a.state.sessions, a.state.sessionsValid
}

// GetCurrentSessionPeerID returns the mounted session peer ID used for the
// account signer when available.
func (a *ProviderAccount) GetCurrentSessionPeerID() peer.ID {
	cli := a.GetSessionClient()
	if cli == nil {
		return ""
	}
	return cli.GetPeerID()
}

// EmailsSnapshot returns the cached email list.
// Must be called within an accountBcast HoldLock scope.
func (a *ProviderAccount) EmailsSnapshot() ([]*api.AccountEmailInfo, bool) {
	return a.state.cachedEmails, a.state.cachedEmailsValid
}

// GetSubscriptionStatus returns the normalized subscription status string from
// account state. Fetches GET /account/state on cache miss. Returns "" if not
// yet determined.
func (a *ProviderAccount) GetSubscriptionStatus(ctx context.Context) (string, error) {
	state, err := a.GetAccountState(ctx)
	if err != nil {
		return "", err
	}
	return state.GetSubscriptionStatus().NormalizedString(), nil
}

// GetAccountState returns cached account state, fetching GET /account/state on
// cache miss. Uses a fetching flag to coalesce concurrent callers so only one
// HTTP request is made; other goroutines wait on the broadcast for the result.
func (a *ProviderAccount) GetAccountState(ctx context.Context) (*api.AccountStateResponse, error) {
	for {
		var info *api.AccountStateResponse
		var ch <-chan struct{}
		var shouldFetch bool
		var cli *SessionClient
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			info = a.state.info
			cli = a.sessionClient
			if info == nil && !a.state.infoFetching && cli != nil {
				a.state.infoFetching = true
				shouldFetch = true
			}
			ch = getWaitCh()
		})
		if info != nil {
			return info, nil
		}
		if shouldFetch {
			fetched, err := cli.GetAccountState(ctx)
			if err != nil {
				a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
					a.state.infoFetching = false
					broadcast()
				})
				return nil, err
			}
			a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				a.state.info = fetched
				a.state.status = loadedAccountStatus(fetched)
				a.state.infoFetching = false
				broadcast()
			})
			a.refreshSelfRejoinSweepState()
			return fetched, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

// DeleteAccount sends a signed account deletion request to the cloud.
// entityKeys and entityPeerIDs are the multi-sig entity keys for the account.
func (a *ProviderAccount) DeleteAccount(
	ctx context.Context,
	entityKeys []crypto.PrivKey,
	entityPeerIDs []string,
) error {
	_, err := a.entityCli.DeleteAccount(ctx, a.accountID, entityKeys, entityPeerIDs)
	return err
}

// GetProviderAccountFeature returns the implementation of a specific provider feature.
func (a *ProviderAccount) GetProviderAccountFeature(ctx context.Context, feature provider.ProviderFeature) (provider.ProviderAccountFeature, error) {
	switch feature {
	case provider.ProviderFeature_ProviderFeature_BLOCK_STORE:
		return bstore.BlockStoreProvider(a), nil
	case provider.ProviderFeature_ProviderFeature_SHARED_OBJECT:
		return sobject.SharedObjectProvider(a), nil
	case provider.ProviderFeature_ProviderFeature_SHARED_OBJECT_RECOVERY:
		return sobject.SharedObjectRecoveryProvider(a), nil
	case provider.ProviderFeature_ProviderFeature_SESSION:
		return session.SessionProvider(a), nil
	default:
		return nil, provider.ErrUnimplementedProviderFeature
	}
}

// GetStorageStats returns storage usage statistics for the account volume.
func (a *ProviderAccount) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	return a.vol.GetStorageStats(ctx)
}

// _ is a type assertion
var (
	_ provider.ProviderAccount      = ((*ProviderAccount)(nil))
	_ provider.StorageStatsProvider = ((*ProviderAccount)(nil))
)

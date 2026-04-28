package provider_local

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

// providerAccountTracker tracks a ProviderAccount in the world.
type providerAccountTracker struct {
	// p is the provider
	p *Provider
	// accCtr is the provider account container
	accCtr *ccontainer.CContainer[*ProviderAccount]
	// accountInfo is the account info to create if not exists in the world
	accountInfo *provider.ProviderAccountInfo
}

// ProviderAccount implements the local provider account.
type ProviderAccount struct {
	// t is the tracker
	t *providerAccountTracker
	// le is the logger
	le *logrus.Entry
	// vol is the parent volume for storage for the account
	vol volume.Volume

	// bstores contains the set of mounted block stores.
	bstores *keyed.KeyedRefCount[string, *bstoreTracker]
	// sobjects contains the set of mounted shared objects.
	sobjects *keyed.KeyedRefCount[string, *sobjectTracker]
	// sessions contains the set of mounted sessions (usually only one).
	sessions *keyed.KeyedRefCount[string, *sessionTracker]

	// mtx guards /changing/ below fields
	mtx csync.Mutex
	// soListCtr is the list of shared objects.
	soListCtr *ccontainer.CContainer[*sobject.SharedObjectList]
	// p2pSyncMtx guards p2pSync lifecycle.
	p2pSyncMtx sync.Mutex
	// p2pSync holds running P2P sync state, nil when not active.
	p2pSync *p2pSyncState
	// sessionTransport is the running session transport, nil when not active.
	sessionTransport *sessionTransportState
	// transportBcast guards sessionTransport state changes.
	transportBcast broadcast.Broadcast
	// accountSettingsCloudSync mirrors local account settings to a linked cloud
	// account settings SO when a linked cloud account is available.
	accountSettingsCloudSync *routine.StateRoutineContainer[string]
	// accountSettingsProcessor processes account settings operations.
	accountSettingsProcessor *routine.RoutineContainer
	// envelopeRewrapWatcher watches for envelope rewrap work.
	envelopeRewrapWatcher *routine.RoutineContainer
	// orgProcessors watches org SO membership and runs org processors.
	orgProcessors *routine.RoutineContainer
	// pairing tracks an active pairing flow, nil when not active.
	pairing *pairingState
	// pairingCtx is the ProviderAccount lifecycle context for pairing routines.
	pairingCtx context.Context
	// pairingBcast guards pairing state changes.
	pairingBcast broadcast.Broadcast
}

// GetVolume returns the parent volume for the account.
func (a *ProviderAccount) GetVolume() volume.Volume {
	return a.vol
}

// GetAccountID returns the provider account identifier.
func (a *ProviderAccount) GetAccountID() string {
	return a.t.accountInfo.GetProviderAccountId()
}

// GetProviderID returns the provider identifier.
func (a *ProviderAccount) GetProviderID() string {
	return a.t.accountInfo.GetProviderId()
}

// GetSOListCtr returns the shared object list container.
func (a *ProviderAccount) GetSOListCtr() *ccontainer.CContainer[*sobject.SharedObjectList] {
	return a.soListCtr
}

// GetStepFactorySet returns the block transform step factory set.
func (a *ProviderAccount) GetStepFactorySet() *block_transform.StepFactorySet {
	return a.t.p.sfs
}

// NewProviderAccountInfo constructs a new provider account info object for the local provider.
func NewProviderAccountInfo(providerID, accountID string) *provider.ProviderAccountInfo {
	return &provider.ProviderAccountInfo{
		ProviderId:            providerID,
		ProviderAccountId:     accountID,
		ProviderFeatures:      getLocalProviderFeatures(),
		ProviderAccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		ProviderAccountState:  nil,
	}
}

// buildProviderAccountTracker builds a new providerAccountTracker for an account id.
func (p *Provider) buildProviderAccountTracker(accountID string) (keyed.Routine, *providerAccountTracker) {
	accCtr := ccontainer.NewCContainer[*ProviderAccount](nil)
	tracker := &providerAccountTracker{
		p:           p,
		accCtr:      accCtr,
		accountInfo: NewProviderAccountInfo(p.info.GetProviderId(), accountID),
	}
	return tracker.executeProviderAccountTracker, tracker
}

// executeProviderAccountTracker executes the provider account tracker routine.
func (t *providerAccountTracker) executeProviderAccountTracker(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Look up or create the ProviderAccount in the world.
	providerID := t.p.info.GetProviderId()
	accountID := t.accountInfo.GetProviderAccountId()

	// Mount the storage volume for this space.
	storageID := t.p.storageID
	if storageID == "" {
		storageID = "default"
	}

	// Storage volume id
	storageVolumeID := StorageVolumeID(providerID, accountID)
	volumeID := storageVolumeID

	// Start the storage volume controller.
	volCtrl, _, volCtrlRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		t.p.b,
		resolver.NewLoadControllerWithConfig(&storage_volume.Config{
			StorageId:       storageID,
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

	// wait for the volume
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Construct the ProviderAccount handle
	le := t.p.le.WithField("account-id", t.accountInfo.GetProviderAccountId())
	providerAcc := &ProviderAccount{
		t:   t,
		vol: vol,
		le:  le,
	}

	providerAcc.bstores = keyed.NewKeyedRefCountWithLogger(
		providerAcc.buildBlockStoreTracker,
		t.p.le,
		keyed.WithRetry[string, *bstoreTracker](providerBackoff),
	)
	providerAcc.sobjects = keyed.NewKeyedRefCountWithLogger(
		providerAcc.buildSharedObjectTracker,
		t.p.le,
		keyed.WithRetry[string, *sobjectTracker](providerBackoff),
	)
	providerAcc.sessions = keyed.NewKeyedRefCountWithLogger(
		providerAcc.buildSessionTracker,
		t.p.le,
		keyed.WithRetry[string, *sessionTracker](providerBackoff),
	)
	providerAcc.accountSettingsCloudSync = routine.NewStateRoutineContainerWithLogger[string](
		func(v1, v2 string) bool { return v1 == v2 },
		le.WithField("routine", "account-settings-cloud-sync"),
		routine.WithRetry(providerBackoff),
	)
	providerAcc.accountSettingsCloudSync.SetStateRoutine(providerAcc.runAccountSettingsCloudSync)
	providerAcc.accountSettingsProcessor = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "account-settings-processor"),
		routine.WithRetry(providerBackoff),
	)
	providerAcc.accountSettingsProcessor.SetRoutine(providerAcc.runAccountSettingsProcessor)
	providerAcc.envelopeRewrapWatcher = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "envelope-rewrap-watcher"),
		routine.WithRetry(providerBackoff),
	)
	providerAcc.envelopeRewrapWatcher.SetRoutine(providerAcc.watchAndRewrapEnvelope)
	providerAcc.orgProcessors = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "org-processors"),
		routine.WithRetry(providerBackoff),
	)
	providerAcc.orgProcessors.SetRoutine(providerAcc.watchOrgProcessors)

	// initialize the shared object list
	providerAcc.soListCtr = ccontainer.NewCContainer[*sobject.SharedObjectList](nil)

	// Load the shared object soList
	soList, err := providerAcc.readSharedObjectList(ctx)
	if err != nil {
		return err
	}
	providerAcc.soListCtr.SetValue(soList)

	// Ensure the account settings binding exists.
	if _, err := providerAcc.EnsureAccountSettingsSO(ctx); err != nil {
		return err
	}

	// Start the block stores tracker
	providerAcc.bstores.SetContext(ctx, true)
	defer providerAcc.bstores.ClearContext()

	// Start the shared objects tracker
	providerAcc.sobjects.SetContext(ctx, true)
	defer providerAcc.sobjects.ClearContext()

	// Start the sessions tracker
	providerAcc.sessions.SetContext(ctx, true)
	defer providerAcc.sessions.ClearContext()
	if linkedCloudAccountID, err := providerAcc.loadLinkedCloudAccountID(ctx); err != nil {
		return err
	} else {
		providerAcc.accountSettingsCloudSync.SetState(linkedCloudAccountID)
	}
	providerAcc.accountSettingsCloudSync.SetContext(ctx, true)
	defer providerAcc.accountSettingsCloudSync.ClearContext()
	providerAcc.accountSettingsProcessor.SetContext(ctx, true)
	defer providerAcc.accountSettingsProcessor.ClearContext()
	providerAcc.envelopeRewrapWatcher.SetContext(ctx, true)
	defer providerAcc.envelopeRewrapWatcher.ClearContext()
	providerAcc.orgProcessors.SetContext(ctx, true)
	defer providerAcc.orgProcessors.ClearContext()

	// Cleanup on exit.
	providerAcc.setPairingContext(ctx)
	defer providerAcc.setPairingContext(nil)
	defer providerAcc.ClearPairingState()
	defer providerAcc.StopSessionTransport()
	defer providerAcc.StopP2PSync()

	// Startup complete
	t.accCtr.SetValue(providerAcc)
	defer t.accCtr.SetValue(nil)

	<-ctx.Done()
	return context.Canceled
}

// GetProviderAccountFeature returns the implementation of a specific provider feature.
//
// Implements one of SpaceProvider, BlockStoreProvider, ...
// Check GetProviderInfo()=>features in advance before calling this.
// Returns ErrUnimplementedProviderFeature if the feature is not implemented.
func (a *ProviderAccount) GetProviderAccountFeature(ctx context.Context, feature provider.ProviderFeature) (provider.ProviderAccountFeature, error) {
	switch feature {
	case provider.ProviderFeature_ProviderFeature_BLOCK_STORE:
		return bstore.BlockStoreProvider(a), nil
	case provider.ProviderFeature_ProviderFeature_SHARED_OBJECT:
		return sobject.SharedObjectProvider(a), nil
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

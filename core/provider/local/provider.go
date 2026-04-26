package provider_local

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Provider implements the local provider.
type Provider struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// storageID is the storage controller to use
	storageID string
	// info is the provider info
	info *provider.ProviderInfo
	// sfs is the step factory set for block transforms
	sfs *block_transform.StepFactorySet

	// accountRc is the keyed refcount for accounts.
	accountRc *keyed.KeyedRefCount[string, *providerAccountTracker]
	// peer is the peer instance
	peer peer.Peer
	// handler is the provider handler
	handler provider.ProviderHandler
}

// providerBackoff is the default backoff for provider services.
var providerBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 500,
		MaxInterval:     1800,
		Multiplier:      1.4,
	},
}

// NewProvider constructs a new Provider.
func NewProvider(
	le *logrus.Entry,
	b bus.Bus,
	storageID string,
	info *provider.ProviderInfo,
	peer peer.Peer,
	handler provider.ProviderHandler,
) *Provider {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_s2.NewStepFactory())
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())

	p := &Provider{
		le:        le,
		b:         b,
		storageID: storageID,
		info:      info,
		peer:      peer,
		handler:   handler,
		sfs:       sfs,
	}
	p.accountRc = keyed.NewKeyedRefCountWithLogger(
		p.buildProviderAccountTracker,
		le,
		keyed.WithRetry[string, *providerAccountTracker](&backoff.Backoff{}),
	)
	return p
}

// getLocalProviderFeatures returns the slice of provider features implemented by the local provider.
func getLocalProviderFeatures() []provider.ProviderFeature {
	return []provider.ProviderFeature{
		provider.ProviderFeature_ProviderFeature_SESSION,
		provider.ProviderFeature_ProviderFeature_SHARED_OBJECT,
		provider.ProviderFeature_ProviderFeature_BLOCK_STORE,
	}
}

// NewProviderInfo constructs the provider info.
func NewProviderInfo(providerID string) *provider.ProviderInfo {
	return &provider.ProviderInfo{
		ProviderId:       providerID,
		ProviderFeatures: getLocalProviderFeatures(),
	}
}

// GetProviderInfo returns the basic provider information.
func (p *Provider) GetProviderInfo() *provider.ProviderInfo {
	return p.info.CloneVT()
}

// CreateLocalAccountAndSession initializes a local provider account and session.
// cloudAccountID links this local session to a cloud account (empty for standalone).
//
// NOTE: this is a WIP / possibly temporary function.
func (p *Provider) CreateLocalAccountAndSession(ctx context.Context, cloudAccountID string) (*session.SessionRef, error) {
	// Generate an ID for the local account and session.
	localAccountID := ulid.NewULID()
	localSessionID := ulid.NewULID()

	// Create the provider account.
	// For the local provider, the account is created on first mount.
	provAcc, relProvAcc, err := p.AccessProviderAccount(ctx, localAccountID, nil)
	if err != nil {
		return nil, err
	}
	defer relProvAcc()

	// Mount the session. For the local provider this also inits on first run.
	sessRef := &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                localSessionID,
			ProviderAccountId: localAccountID,
			ProviderId:        p.info.GetProviderId(),
		},
	}

	// Access the tracker directly to set cloudAccountID before the session
	// starts executing (the tracker blocks on ref.Await until we set it).
	localAcc := provAcc.(*ProviderAccount)
	tkrRef, tkr, _ := localAcc.sessions.AddKeyRef(localSessionID)
	tkr.cloudAccountID = cloudAccountID
	tkr.ref.SetResult(sessRef, nil)

	_, err = tkr.sessionProm.Await(ctx)
	if err != nil {
		tkrRef.Release()
		return nil, err
	}
	if cloudAccountID != "" {
		if err := localAcc.writeLinkedCloudAccountID(ctx, localSessionID, cloudAccountID); err != nil {
			tkrRef.Release()
			return nil, err
		}
	}
	tkrRef.Release()

	return sessRef, nil
}

func (a *ProviderAccount) writeLinkedCloudAccountID(ctx context.Context, sessionID, cloudAccountID string) error {
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		a.t.p.b,
		false,
		SessionObjectStoreID(a.GetProviderID(), a.GetAccountID()),
		a.vol.GetID(),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()

	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "open session object store transaction")
	}
	defer otx.Discard()

	if err := otx.Set(ctx, LinkedCloudKey(sessionID), []byte(cloudAccountID)); err != nil {
		return errors.Wrap(err, "set linked cloud account id")
	}
	if err := otx.Commit(ctx); err != nil {
		return errors.Wrap(err, "commit linked cloud account id")
	}
	return nil
}

// AccessProviderAccount accesses a provider account.
// If accountID is empty, it will use the default or prompt the user.
// released may be nil
func (p *Provider) AccessProviderAccount(ctx context.Context, accountID string, released func()) (provider.ProviderAccount, func(), error) {
	ref, providerAccTkr, _ := p.accountRc.AddKeyRef(accountID)
	providerAcc, err := providerAccTkr.accCtr.WaitValue(ctx, nil)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}

	return providerAcc, ref.Release, nil
}

// Execute executes the provider.
// Return nil for no-op (will not be restarted).
func (p *Provider) Execute(ctx context.Context) error {
	p.accountRc.SetContext(ctx, true)
	return nil
}

// _ is a type assertion
var _ provider.Provider = (*Provider)(nil)

package provider_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a provider with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	info *provider.ProviderInfo,
	peer peer.Peer,
	handler provider.ProviderHandler,
) (provider.Provider, error)

// ProviderController is the common implementation of the provider controller.
type ProviderController struct {
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor
	// info is the controller info
	info *controller.Info
	// providerInfo is the provider info
	providerInfo *provider.ProviderInfo
	// lookupPeerID is the peer id to lookup on the bus
	// may be empty
	lookupPeerID peer.ID
	// providerCtr contains the provider initialized by Execute
	providerCtr *ccontainer.CContainer[provider.Provider]
}

// NewProviderController constructs a new provider controller.
func NewProviderController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	providerInfo *provider.ProviderInfo,
	peerID peer.ID,
	ctor Constructor,
) *ProviderController {
	return &ProviderController{
		le:           le,
		bus:          bus,
		info:         info,
		providerInfo: providerInfo,
		lookupPeerID: peerID,
		ctor:         ctor,
		providerCtr:  ccontainer.NewCContainer[provider.Provider](nil),
	}
}

// GetControllerID returns the controller ID.
func (c *ProviderController) GetControllerID() string {
	return c.info.GetId()
}

// GetControllerInfo returns information about the controller.
func (c *ProviderController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// GetProvider returns the provider, waiting for it to be ready.
//
// Returns nil, context.Canceled if canceled.
func (c *ProviderController) GetProvider(ctx context.Context) (provider.Provider, error) {
	return c.providerCtr.WaitValue(ctx, nil)
}

// GetProviderInfo returns the provider information
func (c *ProviderController) GetProviderInfo() *provider.ProviderInfo {
	return c.providerInfo.CloneVT()
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *ProviderController) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	localPeer, _, localPeerRef, err := peer.GetPeerWithID(ctx, c.bus, c.lookupPeerID, false, ctxCancel)
	if err != nil {
		return err
	}
	defer localPeerRef.Release()

	// validate provider info
	if err := c.providerInfo.Validate(); err != nil {
		return errors.Wrap(err, "invalid provider info")
	}

	// Construct the provider instance
	prov, err := c.ctor(ctx, c.le, c.providerInfo, localPeer, c)
	if err != nil {
		return err
	}
	c.providerCtr.SetValue(prov)

	// Execute the provider (if applicable)
	err = prov.Execute(ctx)
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return err
	}

	<-ctx.Done()
	return context.Canceled
}

// HandleDirective asks if the handler can resolve the directive.
//
// Resolves LookupProvider, LookupProviderInfo.
func (c *ProviderController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case provider.LookupProvider:
		if dir.LookupProviderID() == "" || dir.LookupProviderID() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewGetterResolver(c.GetProvider), nil)
		}

	case provider.LookupProviderInfo:
		if dir.LookupProviderInfoID() == "" || dir.LookupProviderInfoID() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewValueResolver([]provider.LookupProviderInfoValue{c.GetProviderInfo()}), nil)
		}

	case provider.AccessProviderAccount:
		if dir.AccessProviderID() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (provider.AccessProviderAccountValue, func(), error) {
				prov, err := c.GetProvider(ctx)
				if err != nil {
					return nil, nil, err
				}

				return prov.AccessProviderAccount(ctx, dir.AccessProviderAccountID(), released)
			}), nil)
		}

	case bstore.MountBlockStore:
		if ref := dir.MountBlockStoreRef(); ref.GetProviderResourceRef().GetProviderId() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (bstore.MountBlockStoreValue, func(), error) {
				prov, err := c.GetProvider(ctx)
				if err != nil {
					return nil, nil, err
				}

				provAcc, relProvAcc, err := prov.AccessProviderAccount(ctx, ref.GetProviderResourceRef().GetProviderAccountId(), released)
				if err != nil {
					return nil, nil, err
				}

				bstoreAccFeature, err := bstore.GetBlockStoreProviderAccountFeature(ctx, provAcc)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				so, soRel, err := bstoreAccFeature.MountBlockStore(ctx, ref, released)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				return so, func() {
					soRel()
					relProvAcc()
				}, nil
			}), nil)
		}

	case sobject.MountSharedObject:
		if ref := dir.MountSharedObjectRef(); ref.GetProviderResourceRef().GetProviderId() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (sobject.MountSharedObjectValue, func(), error) {
				prov, err := c.GetProvider(ctx)
				if err != nil {
					return nil, nil, err
				}

				provAcc, relProvAcc, err := prov.AccessProviderAccount(ctx, ref.GetProviderResourceRef().GetProviderAccountId(), released)
				if err != nil {
					return nil, nil, err
				}

				provAccFeature, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				so, soRel, err := provAccFeature.MountSharedObject(ctx, ref, released)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				return so, func() {
					soRel()
					relProvAcc()
				}, nil
			}), nil)
		}
	case session.MountSession:
		if ref := dir.MountSessionRef(); ref.GetProviderResourceRef().GetProviderId() == c.providerInfo.GetProviderId() {
			return directive.R(directive.NewAccessResolver(func(ctx context.Context, released func()) (session.MountSessionValue, func(), error) {
				prov, err := c.GetProvider(ctx)
				if err != nil {
					return nil, nil, err
				}

				provAcc, relProvAcc, err := prov.AccessProviderAccount(ctx, ref.GetProviderResourceRef().GetProviderAccountId(), released)
				if err != nil {
					return nil, nil, err
				}

				provAccFeature, err := session.GetSessionProviderAccountFeature(ctx, provAcc)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				so, soRel, err := provAccFeature.MountSession(ctx, ref, released)
				if err != nil {
					relProvAcc()
					return nil, nil, err
				}

				return so, func() {
					soRel()
					relProvAcc()
				}, nil
			}), nil)
		}
	}
	return nil, nil
}

// Close closes the provider controller.
func (c *ProviderController) Close() error {
	c.providerCtr.SetValue(nil)
	return nil
}

// _ is a type assertion
var _ provider.ProviderController = (*ProviderController)(nil)

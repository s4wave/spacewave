package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	sharedobject_dexaccess "github.com/s4wave/spacewave/core/sobject/dexaccess"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/peer"
)

// authorizeDEXRequest acquires held authority for exactly the requested object.
func (a *ProviderAccount) authorizeDEXRequest(ctx context.Context, config *dex_solicit.Config, requester peer.ID) (dex_solicit.RequestAccess, error) {
	// Resolve only a held object, without treating its retained listing as a grant.
	object, releaseObject, err := a.mountListedSharedObject(ctx, string(config.GetProtocolContext()))
	if err != nil {
		return nil, err
	}
	if object == nil {
		return nil, sobject.ErrSharedObjectNotFound
	}

	// Retain the authority source independently of SO sync and hosting lifetimes.
	return sharedobject_dexaccess.NewSharedObjectRequestAccess(ctx, object.ctx, object.soHost, requester, releaseObject)
}

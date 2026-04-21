package identity_domain_service

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/crypto"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
)

// LookupEntity looks up an entity by identifier.
//
// returns nil, nil on not found
func LookupEntity(
	ctx context.Context,
	cl stream_srpc_client.Client,
	localPriv crypto.PrivKey,
	domainID, entityID string,
) (*identity.Entity, error) {
	svc := NewSRPCIdentityDomainClient(cl)

	req, err := NewLookupEntityReq(domainID, entityID, nil, 0)
	if err != nil {
		return nil, err
	}

	sigReq, err := req.SignReq(localPriv)
	if err != nil {
		return nil, err
	}

	resp, err := svc.LookupEntity(ctx, sigReq)
	if err != nil {
		return nil, err
	}
	if resp.GetNotFound() {
		return nil, nil
	}
	lookupErr := resp.GetLookupError()
	if len(lookupErr) != 0 {
		return nil, errors.New(lookupErr)
	}
	return resp.GetLookupEntity(), nil
}

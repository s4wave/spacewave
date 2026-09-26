package identity_domain_service

import (
	"context"

	"github.com/pkg/errors"
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
	// Construct the identity-domain client and lookup request.
	svc := NewSRPCIdentityDomainClient(cl)

	req, err := NewLookupEntityReq(domainID, entityID, nil, 0)
	if err != nil {
		return nil, err
	}

	// Sign the lookup request for the remote domain service.
	sigReq, err := req.SignReq(localPriv)
	if err != nil {
		return nil, err
	}

	// Submit the request and translate the response into an entity result.
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
	ent := resp.GetLookupEntity()
	if err := ValidateLookupEntity(ent, domainID, entityID); err != nil {
		return nil, err
	}
	return ent, nil
}

// ValidateLookupEntity checks that a looked-up entity is valid and matches the
// requested domain and entity ID.
func ValidateLookupEntity(ent *identity.Entity, domainID, entityID string) error {
	if ent == nil {
		return errors.New("lookup returned empty entity")
	}
	if got := ent.GetDomainId(); got != domainID {
		return errors.Errorf("lookup returned domain id %q but expected %q", got, domainID)
	}
	if got := ent.GetEntityId(); got != entityID {
		return errors.Errorf("lookup returned entity id %q but expected %q", got, entityID)
	}
	if err := ent.Validate(); err != nil {
		return errors.Wrap(err, "lookup returned invalid entity")
	}
	return nil
}

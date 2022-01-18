package identity_domain

import (
	"context"
	"errors"

	stream_drpc_client "github.com/aperturerobotics/bifrost/stream/drpc/client"
	"github.com/aperturerobotics/identity"
	"github.com/libp2p/go-libp2p-core/crypto"
	"storj.io/drpc/drpcconn"
)

// LookupEntity looks up an entity by identifier.
//
// returns nil, nil on not found
func LookupEntity(
	ctx context.Context,
	cl *stream_drpc_client.Client,
	localPriv crypto.PrivKey,
	entityID, domainID string,
) (*identity.Entity, error) {
	var entity *identity.Entity
	err := cl.ExecuteConnection(
		ctx,
		IdentityDomainProtocol,
		func(conn *drpcconn.Conn) (next bool, err error) {
			svc := NewDRPCIdentityDomainClient(conn)

			req, err := NewLookupEntityReq(entityID, domainID, nil, 0)
			if err != nil {
				return false, err
			}

			sigReq, err := req.SignReq(localPriv)
			if err != nil {
				return false, err
			}

			resp, err := svc.LookupEntity(ctx, sigReq)
			if err != nil {
				// try the next server
				return true, err
			}
			if !resp.GetNotFound() {
				lookupErr := resp.GetLookupError()
				if len(lookupErr) != 0 {
					return false, errors.New(lookupErr)
				}
				entity = resp.GetLookupEntity()
			}
			return false, nil
		},
	)
	return entity, err
}

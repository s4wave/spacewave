package resource_session

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
	stream_api "github.com/s4wave/spacewave/net/stream/api"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// AccessPeerTransport exposes the mounted local account's authenticated stream
// transport. The account owns the transport; each stream belongs to its caller.
func (r *SessionResource) AccessPeerTransport(
	ctx context.Context,
	_ *s4wave_session.AccessPeerTransportRequest,
) (*s4wave_session.AccessPeerTransportResponse, error) {
	owner, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	account, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("peer transport requires a local account")
	}
	if err := account.EnsureConfiguredSessionTransport(ctx, r.session.GetPrivKey()); err != nil {
		return nil, err
	}
	transport := account.GetSessionTransport()
	if transport == nil || transport.GetChildBus() == nil {
		return nil, errors.New("account peer transport is unavailable")
	}
	api, err := bifrost_api.NewAPI(transport.GetChildBus(), &bifrost_api.Config{})
	if err != nil {
		return nil, err
	}
	mux := srpc.NewMux()
	if err := stream_api.SRPCRegisterStreamService(mux, api); err != nil {
		return nil, err
	}
	id, err := owner.AddResource(mux, nil)
	if err != nil {
		return nil, err
	}
	return &s4wave_session.AccessPeerTransportResponse{
		ResourceId: id,
		PeerId:     transport.GetPeerID().String(),
	}, nil
}

// sharedObjectTransportPeer returns the verified endpoint retained by enrollment.
func (r *SessionResource) sharedObjectTransportPeer(sharedObjectID string) string {
	account, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return ""
	}
	for _, entry := range account.GetSOListCtr().GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == sharedObjectID {
			return entry.GetTransportPeerId()
		}
	}
	return ""
}

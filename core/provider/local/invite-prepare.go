package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// PrepareDirectInvite binds an invitation to the account's live transport and
// keeps its invitation server running for the account lifetime.
func (a *ProviderAccount) PrepareDirectInvite(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	ownerKey crypto.PrivKey,
	invite *sobject.SOInviteMessage,
) error {
	if err := a.EnsureConfiguredSessionTransport(ctx, sessionKey); err != nil {
		return errors.Wrap(err, "start invitation transport")
	}
	transport := a.GetSessionTransport()
	if transport == nil {
		return errors.New("invitation transport is unavailable")
	}
	if err := a.StartPersistentP2PSync(ctx, transport); err != nil {
		return errors.Wrap(err, "start invitation server")
	}
	if target := invite.GetTargetPeerId(); target != "" {
		targetPeer, err := peer.IDB58Decode(target)
		if err != nil {
			return errors.Wrap(err, "parse invitation recipient")
		}
		if err := a.RetainP2PPeer(ctx, targetPeer); err != nil {
			return errors.Wrap(err, "retain invitation recipient")
		}
	}

	// The storage owner signs the route; it does not impersonate the session peer.
	invite.TransportPeerId = transport.GetPeerID().String()
	return invite.Sign(ownerKey)
}

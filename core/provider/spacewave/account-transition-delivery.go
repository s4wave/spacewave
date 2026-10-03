package provider_spacewave

import (
	"context"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// EnrollMigratedSession keeps cloud enrollment under the provider registry's
// signed migration endpoint; a direct peer cannot register cloud credentials.
func (a *ProviderAccount) EnrollMigratedSession(context.Context, *provider_migration.MigratedSessionRequest) (*provider_migration.MigratedSessionResponse, error) {
	return nil, errors.New("cloud Session enrollment requires provider authorization")
}

// DeliverAccountTransition acknowledges recovery already completed under this
// cloud account. The provider's accepted transition is the authority; a replayed
// checkpoint cannot change any resource, credential, or account attachment.
func (a *ProviderAccount) DeliverAccountTransition(ctx context.Context, checkpoint *pairing.SharedObject) (*provider_migration.AccountTransitionReceipt, error) {
	stream, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return nil, err
	}
	var accepted []*provider.AccountTransition
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		accepted = a.state.info.GetAcceptedMigrations()
	})
	for _, transition := range accepted {
		if transition.GetDestination().GetProviderAccountId() == a.accountID && transition.GetDestinationEndpoint() == a.p.endpoint && transition.GetSource().GetId() == checkpoint.GetEntry().GetRef().GetProviderResourceRef().GetId() && (slices.Contains(transition.GetDestinationPeerIds(), stream.GetPeerID().String()) || slices.Contains(transition.GetSessionPeerIds(), stream.GetPeerID().String())) {
			return &provider_migration.AccountTransitionReceipt{}, nil
		}
	}
	return nil, errors.New("cloud Session has not accepted this account transition")
}

// deliverAccountTransitions retains signed source checkpoints and their pending
// Session endpoints. A successful acknowledgment means the source replica has
// durably accepted its redirect; no further delivery is needed in this lifetime.
func (a *ProviderAccount) deliverAccountTransitions(ctx context.Context, transport *transport.SessionTransport) error {
	retained := make(map[string]directive.Reference)
	defer func() {
		for _, reference := range retained {
			reference.Release()
		}
	}()
	delivered := make(map[string]bool)
	for {
		var transitions []*provider.AccountTransition
		var accountChanged <-chan struct{}
		a.accountBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			transitions = a.state.info.GetAcceptedMigrations()
			accountChanged = getWait()
		})
		connections, changed := transport.GetLinkSnapshotsWithWait()
		for _, transition := range transitions {
			if transition.GetSourceEndpoint() != "" {
				continue
			}
			if err := transition.Validate(); err != nil {
				return err
			}
			for _, id := range transition.GetSessionPeerIds() {
				if id == transport.GetPeerID().String() || retained[id] != nil {
					continue
				}
				remote, _, err := peer.ParsePeerIDWithPubKey(id)
				if err != nil {
					return err
				}
				handler := directive.NewTypedCallbackHandler[link.MountedLink](func(directive.TypedAttachedValue[link.MountedLink]) {}, nil, nil, nil)
				_, reference, err := transport.GetChildBus().AddDirective(link.NewEstablishLinkWithPeer(transport.GetPeerID(), remote), handler)
				if err != nil {
					return err
				}
				retained[id] = reference
			}
			var checkpoint *pairing.SharedObject
			for _, connection := range connections {
				remote := connection.RemotePeerID
				key := transition.GetOperationId() + "/" + remote.String()
				if delivered[key] || !slices.Contains(transition.GetSessionPeerIds(), remote.String()) {
					continue
				}
				if checkpoint == nil {
					var err error
					checkpoint, err = a.accountRecoveryCheckpoint(ctx, transition)
					if err != nil {
						return err
					}
				}
				requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				open := stream_srpc.NewOpenStreamFunc(transport.GetChildBus(), provider_migration.RecoveryProtocol, transport.GetPeerID(), remote, 0)
				_, err := provider_migration.NewSRPCAccountMigrationServiceClient(srpc.NewClient(open)).DeliverAccountTransition(requestCtx, checkpoint)
				cancel()
				if err != nil {
					return err
				}
				delivered[key] = true
			}
		}
		if err := broadcast.WaitAny(ctx, append(changed, accountChanged)...); err != nil {
			return err
		}
	}
}

// accountRecoveryCheckpoint reads the exact resource named by the committed
// account migration. The returning client verifies it against its held history.
func (a *ProviderAccount) accountRecoveryCheckpoint(ctx context.Context, transition *provider.AccountTransition) (*pairing.SharedObject, error) {
	// Mount the source account object.
	id := transition.GetSource().GetId()
	ref := sobject.NewSharedObjectRef(a.GetProviderID(), a.accountID, id, SobjectBlockStoreID(id))
	object, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer release()

	// Return its host state and config lineage.
	cloud := object.(*SharedObject)
	state, err := cloud.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	lineage, err := cloud.GetSOHost().ReadConfigLineage(ctx, state.GetConfig().GetConfigChainHash())
	if err != nil {
		return nil, err
	}
	return &pairing.SharedObject{
		Entry: &sobject.SharedObjectListEntry{Ref: ref, Meta: &sobject.SharedObjectMeta{BodyType: "account-settings", AccountPrivate: true}, Source: "migration-recovery"},
		State: state, ConfigLineage: lineage,
	}, nil
}

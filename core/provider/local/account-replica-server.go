package provider_local

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
)

// accountReplicaProtocol carries checkpoint requests authorized by account membership.
const accountReplicaProtocol = protocol.ID("alpha/account-replica/1")

// FetchObject authorizes the authenticated Session against the current canonical
// account registry. Catalog knowledge alone never permits checkpoint enrollment.
func (a *ProviderAccount) FetchObject(ctx context.Context, request *AccountReplicaObjectRequest) (*pairing.SharedObject, error) {
	// Serialize the enrollment with other replica mutations.
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Require the request to name this account's settings object.
	stream, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	if request.GetSettingsId() != ref.GetProviderResourceRef().GetId() {
		return nil, errors.New("replica request belongs to another account")
	}
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Require the requesting Session to be an active account member.
	member := settings.FindAccountSession(stream.GetPeerID().String())
	if member == nil || member.GetRevoked() {
		return nil, errors.New("replica Session is not an active account member")
	}

	// Enroll the member into the requested catalog object.
	entry := settings.FindCatalogEntry(request.GetObjectId())
	if entry == nil || entry.GetDeleted() {
		return nil, errors.New("requested object is absent from the account catalog")
	}
	return a.enrollAccountMemberObject(ctx, entry.GetEntry(), member)
}

// startAccountReplicaSync attaches service and reconciliation to the captured P2P
// generation bus. Its ordinary stop path joins reconciliation before releasing mounts.
func (a *ProviderAccount) startAccountReplicaSync(state *p2pSyncState) error {
	// Attach the replica and migration services to the transport's bus.
	transport := state.sessionTransport
	server, err := stream_srpc_server.NewServer(
		state.childBus, a.le,
		controller.NewInfo("alpha/account-replica", controller.MustParseVersion("0.0.1"), "account replica service"),
		[]stream_srpc_server.RegisterFn{
			func(mux srpc.Mux) error { return SRPCRegisterAccountReplicaService(mux, a) },
			func(mux srpc.Mux) error { return provider_migration.SRPCRegisterAccountMigrationService(mux, a) },
		},
		[]protocol.ID{accountReplicaProtocol, provider_migration.RecoveryProtocol}, []string{transport.GetPeerID().String()}, false,
	)
	if err != nil {
		return err
	}

	// Retain the service until generation cleanup after all workers have stopped.
	release, err := state.childBus.AddController(state.ctx, server, nil)
	if err != nil {
		return err
	}
	state.addRelease(release)

	// Run replica reconciliation against the settings state.
	reconcile := routine.NewRoutineContainerWithLogger(a.le.WithField("routine", "account-replica"), routine.WithRetry(providerBackoff))
	reconcile.SetRoutine(func(ctx context.Context) error { return a.runAccountReplicaSync(ctx, state) })
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { state.replicaSync = reconcile })
	reconcile.SetContext(state.ctx, false)
	return nil
}

// _ is a type assertion.
var _ SRPCAccountReplicaServiceServer = (*ProviderAccount)(nil)

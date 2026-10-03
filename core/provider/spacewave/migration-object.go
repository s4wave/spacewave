package provider_spacewave

import (
	"bytes"
	"context"
	"path"
	"slices"

	"github.com/pkg/errors"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
)

// ImportMigrationObject preserves the resource ID, signed history, and accepted
// state. Cloud resources retain their existing store; local resources are copied
// and flushed to the provider before account authority may move.
func (a *ProviderAccount) ImportMigrationObject(ctx context.Context, source provider_migration.Account, object sobject.SharedObject, entry *sobject.SharedObjectListEntry, state *sobject.SOState) error {
	// Within one cloud, the resource only changes accounts.
	client, _, _, err := a.getReadySessionClient(ctx)
	if err != nil {
		return err
	}
	id := object.GetSharedObjectID()
	if cloud, ok := source.(*ProviderAccount); ok && cloud.p.endpoint == a.p.endpoint {
		// Publish the source's pending blocks and checkpoint while its current
		// Session still has authority to finish the upload.
		if err := object.GetBlockStore().(*BlockStore).ForceSync(ctx); err != nil {
			return errors.Wrap(err, "flush resource before account transfer")
		}
		_, err := client.TransferResource(ctx, id, "account", a.accountID)
		if err == nil {
			a.BumpLocalEpoch()
		}
		return err
	}

	// Read the signed config history and wait until local writes are durable.
	history, ok := object.(interface {
		ReadSharedObjectFullConfigHistory(context.Context, *sobject.SharedObjectConfig) ([]*sobject.SOConfigChange, error)
	})
	if !ok {
		return errors.New("source cannot supply this Space's signed history")
	}
	changes, err := history.ReadSharedObjectFullConfigHistory(ctx, state.GetConfig())
	if err != nil {
		return err
	}
	if durable, ok := object.(interface{ WaitDurable(context.Context) error }); ok {
		if err := durable.WaitDurable(ctx); err != nil {
			return err
		}
	}

	// Checkpoint a Space so the destination needs only the blocks of its World.
	var world *sobject_world_engine.InnerState
	if entry.GetMeta().GetBodyType() == "space" {
		state, world, err = provider_migration.Checkpoint(ctx, a.le, a.p.b, a.p.sfs, entry.GetRef(), object, state)
		if err != nil {
			return err
		}
	}

	// Encode the latest config change with its key epoch, invites, and
	// recovery envelopes.
	envelopes, err := a.migrationRecoveryEnvelopes(ctx, client, object, state)
	if err != nil {
		return err
	}
	last, err := changes[len(changes)-1].MarshalVT()
	if err != nil {
		return err
	}
	config, err := (&api.PostConfigStateRequest{
		ConfigChange:      last,
		KeyEpoch:          state.CurrentKeyEpoch(),
		Invites:           state.GetInvites(),
		RecoveryEnvelopes: envelopes,
	}).MarshalVT()
	if err != nil {
		return err
	}

	// Encode the checkpoint and the history.
	checkpoint, err := (&api.PostCheckpointRequest{Checkpoint: state.GetCheckpoint()}).MarshalVT()
	if err != nil {
		return err
	}
	chain, err := (&sobject.SOConfigChainResponse{ConfigChanges: changes, KeyEpochs: state.GetKeyEpochs()}).MarshalVT()
	if err != nil {
		return err
	}

	// Name the resource and create it with its state.
	displayName := getSharedObjectDisplayName(entry.GetMeta())
	if displayName == "" && entry.GetMeta().GetBodyType() == "space" {
		displayName = "Untitled Space"
	}
	request, err := buildCreateWithStateRequest(displayName, entry.GetMeta().GetBodyType(), "account", a.accountID, entry.GetMeta().GetAccountPrivate(), config, checkpoint)
	if err != nil {
		return err
	}
	request.ConfigHistory = chain
	body, err := request.MarshalVT()
	if err != nil {
		return err
	}
	if _, err := client.doPostBinary(ctx, path.Join("/api/sobject", id, "create-with-state"), body, nil, SeedReasonMutation); err != nil {
		return err
	}

	// Copy the World of a Space.
	if world != nil {
		store, release, err := a.MountBlockStore(ctx, NewBlockStoreRef(a.GetProviderID(), a.accountID, id), nil)
		if err != nil {
			return err
		}
		defer release()
		if err := provider_migration.CopyWorld(ctx, object, world, store); err != nil {
			return err
		}
		if err := store.(*BlockStore).ForceSync(ctx); err != nil {
			return err
		}
	}

	// Post the operations the checkpoint does not cover.
	for ops := range slices.Chunk(state.GetOps(), 50) {
		if err := client.PostOps(ctx, id, ops); err != nil {
			return err
		}
	}
	a.BumpLocalEpoch()
	return nil
}

// migrationRecoveryEnvelopes gives the destination entity its ordinary recovery
// capability. Unknown external entity keys remain an explicit import blocker.
func (a *ProviderAccount) migrationRecoveryEnvelopes(ctx context.Context, client *SessionClient, object sobject.SharedObject, state *sobject.SOState) ([]*sobject.SOEntityRecoveryEnvelope, error) {
	// Only this account's entity may read the Space.
	roles := listReadableEntityRoles(state.GetConfig())
	if len(roles) == 0 {
		return nil, nil
	}
	if len(roles) != 1 || roles[a.accountID] == sobject.SOParticipantRole_SOParticipantRole_UNKNOWN {
		return nil, errors.New("this Space's external account recovery keys must be available before cloud import")
	}

	// Load the destination account's recovery public keys.
	info, err := client.GetAccountState(ctx)
	if err != nil {
		return nil, err
	}
	public, err := pubKeysFromEntityKeypairs(info.GetKeypairs())
	if err != nil {
		return nil, err
	}
	if len(public) == 0 {
		return nil, errors.New("destination account has no recovery key")
	}

	// Seal the local grant into a recovery envelope for the account.
	host := object.(sobject.InviteHost)
	epoch := state.CurrentKeyEpoch()
	for _, grant := range epoch.GetGrants() {
		if grant.GetPeerId() != object.GetPeerID().String() {
			continue
		}
		inner, err := grant.DecryptInnerData(host.GetPrivKey(), object.GetSharedObjectID())
		if err != nil {
			return nil, err
		}
		envelope, err := sobject.BuildSOEntityRecoveryEnvelope(a.accountID, epoch.GetEpoch(), state.GetConfig(), &sobject.SOEntityRecoveryMaterial{EntityId: a.accountID, Role: roles[a.accountID], GrantInner: inner}, public)
		if err != nil {
			return nil, err
		}
		return []*sobject.SOEntityRecoveryEnvelope{envelope}, nil
	}
	return nil, errors.New("source has no readable grant for this Space")
}

// ReadSharedObjectFullConfigHistory returns the provider's verified immutable chain.
func (s *SharedObject) ReadSharedObjectFullConfigHistory(ctx context.Context, target *sobject.SharedObjectConfig) ([]*sobject.SOConfigChange, error) {
	// Fetch the provider's chain.
	data, err := s.tkr.a.GetSessionClient().GetConfigChain(ctx, s.GetSharedObjectID())
	if err != nil {
		return nil, err
	}
	chain := &sobject.SOConfigChainResponse{}
	if err := chain.UnmarshalVT(data); err != nil {
		return nil, err
	}

	// Trim it to the target and verify it from genesis.
	changes := chain.GetConfigChanges()
	for len(changes) > 0 && changes[len(changes)-1].GetConfigSeqno() > target.GetConfigChainSeqno() {
		changes = changes[:len(changes)-1]
	}
	if err := sobject.VerifyConfigChain(s.GetSharedObjectID(), changes); err != nil {
		return nil, err
	}

	// The last entry must be the accepted configuration.
	last := changes[len(changes)-1]
	hash, err := sobject.HashSOConfigChange(last)
	if err != nil {
		return nil, err
	}
	if last.GetConfigSeqno() != target.GetConfigChainSeqno() || !bytes.Equal(hash, target.GetConfigChainHash()) {
		return nil, errors.New("provider history does not reach the accepted configuration")
	}
	return changes, nil
}

// ReadSharedObjectConfigHistory exposes the same lineage to a local destination.
func (s *SharedObject) ReadSharedObjectConfigHistory(ctx context.Context, target *sobject.SharedObjectConfig) (*sobject.SharedObjectConfig, []*sobject.SOConfigChange, error) {
	changes, err := s.ReadSharedObjectFullConfigHistory(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	base := changes[0].GetConfig().CloneVT()
	base.ConfigChainSeqno = 0
	base.ConfigChainHash, err = sobject.HashSOConfigChange(changes[0])
	return base, changes[1:], err
}

// ReadSharedObjectGenesis preserves the original bootstrap when moving locally.
func (s *SharedObject) ReadSharedObjectGenesis(ctx context.Context, base *sobject.SharedObjectConfig) (*sobject.SOConfigChange, error) {
	changes, err := s.ReadSharedObjectFullConfigHistory(ctx, base)
	if err != nil {
		return nil, err
	}
	return changes[0], nil
}

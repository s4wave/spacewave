package provider_local

import (
	"context"
	"encoding/hex"
	"slices"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/pairing"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/link"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
)

// DeliverAccountTransition accepts only history descending from this account's
// held authority. The new destination peer is authenticated by that history;
// its transport identity alone grants no access and receives no account data.
func (a *ProviderAccount) DeliverAccountTransition(ctx context.Context, checkpoint *pairing.SharedObject) (*provider_migration.AccountTransitionReceipt, error) {
	stream, err := link.MustGetMountedStreamContext(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return nil, err
	}
	if checkpoint.GetEntry().GetRef().GetProviderResourceRef().GetId() != ref.GetProviderResourceRef().GetId() {
		settings, err := a.readAccountSettings(ctx)
		if err != nil {
			return nil, err
		}
		for _, accepted := range settings.GetAcceptedMigrations() {
			if accepted.GetDestination().EqualVT(ref.GetProviderResourceRef()) && accepted.GetSource().GetId() == checkpoint.GetEntry().GetRef().GetProviderResourceRef().GetId() && (slices.Contains(accepted.GetDestinationPeerIds(), stream.GetPeerID().String()) || slices.Contains(accepted.GetSessionPeerIds(), stream.GetPeerID().String())) {
				// This Session already attached to the destination; the retained
				// source redirect no longer needs to be delivered to it.
				return &provider_migration.AccountTransitionReceipt{}, nil
			}
		}
		return nil, errors.New("account redirect belongs to another settings object")
	}
	if checkpoint.SizeVT() > 10*1024*1024 || len(checkpoint.GetHistory()) > sobject.MaxConfigSuffixEntries {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	object, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	local := object.(*SharedObject)
	current, err := local.soHost.GetHostState(ctx)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]*sobject.SOConfigChange, len(checkpoint.GetHistory()))
	for _, entry := range checkpoint.GetHistory() {
		hash, err := sobject.HashSOConfigChange(entry)
		if err != nil {
			return nil, err
		}
		entries[hex.EncodeToString(hash)] = entry
	}
	suffix, err := sobject.ReadConfigSuffix(ctx, current.GetConfig().GetConfigChainHash(), checkpoint.GetState().GetConfig().GetConfigChainHash(), func(_ context.Context, hash []byte) (*sobject.SOConfigChange, error) {
		return entries[hex.EncodeToString(hash)], nil
	})
	if err != nil {
		return nil, err
	}
	err = local.soHost.ImportPeerSnapshot(ctx, checkpoint.GetState(), suffix, local.GetPeerID(), func(ctx context.Context, state *sobject.SOState) error {
		snapshot := sobject.NewSOStateParticipantHandle(a.le, a.t.p.sfs, local.GetSharedObjectID(), state, local.GetPrivKey(), local.GetPeerID())
		settings, _, err := decodeAccountSettingsSnapshot(ctx, snapshot)
		if err != nil {
			return err
		}
		transition := settings.GetTransition()
		if err := transition.Validate(); err != nil {
			return err
		}
		// The signed redirect names Session transport identities. A migrated
		// Session can deliver it without sharing its old local storage key.
		remote := stream.GetPeerID().String()
		if !transition.GetSource().EqualVT(ref.GetProviderResourceRef()) || (!slices.Contains(transition.GetDestinationPeerIds(), remote) && !slices.Contains(transition.GetSessionPeerIds(), remote)) {
			return errors.New("signed account redirect does not authorize this destination peer")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &provider_migration.AccountTransitionReceipt{}, nil
}

// deliverAccountTransitions sends retained signed redirects when an old Session
// reconnects. It waits on transport events; offline clients create no traffic.
func (a *ProviderAccount) deliverAccountTransitions(ctx context.Context, state *p2pSyncState, settings *account_settings.AccountSettings) error {
	if len(settings.GetAcceptedMigrations()) == 0 {
		return nil
	}
	transport := state.sessionTransport
	for {
		links, wait := transport.GetLinkSnapshotsWithWait()
		for _, transition := range settings.GetAcceptedMigrations() {
			if !slices.Contains(transition.GetDestinationPeerIds(), transport.GetPeerID().String()) && !slices.Contains(transition.GetSessionPeerIds(), transport.GetPeerID().String()) {
				continue
			}
			for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
				if entry.GetSource() != "migration-recovery" || entry.GetRef().GetProviderResourceRef().GetId() != transition.GetSource().GetId() {
					continue
				}
				object, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
				if err != nil {
					return err
				}
				local := object.(*SharedObject)
				checkpoint, err := local.soHost.GetHostState(ctx)
				if err != nil {
					release()
					return err
				}
				base, history, err := local.ReadSharedObjectConfigHistory(ctx, checkpoint.GetConfig())
				if err == nil {
					err = local.soHost.WaitDurable(ctx)
				}
				release()
				if err != nil {
					return err
				}
				message := &pairing.SharedObject{Entry: entry, State: checkpoint, HistoryBase: base, History: history}
				for _, connection := range links {
					remote := connection.RemotePeerID
					if !slices.Contains(transition.GetSessionPeerIds(), remote.String()) || settings.FindAccountSession(remote.String()) != nil {
						continue
					}
					requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					open := stream_srpc.NewOpenStreamFunc(transport.GetChildBus(), provider_migration.RecoveryProtocol, transport.GetPeerID(), remote, 0)
					_, err := provider_migration.NewSRPCAccountMigrationServiceClient(srpc.NewClient(open)).DeliverAccountTransition(requestCtx, message)
					cancel()
					if err != nil {
						return err
					}
				}
			}
		}
		if err := broadcast.WaitAny(ctx, wait...); err != nil {
			return err
		}
	}
}

package provider_local

import (
	"context"
	"time"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// RecordPairedDevice persists a verified Device identity in account settings.
// SpaceLink approval uses it so the originating account restores P2P sync after
// daemon restart without repeating enrollment.
func (a *ProviderAccount) RecordPairedDevice(
	ctx context.Context,
	remotePeerID string,
	displayName string,
) error {
	// Parse the peer ID and mark it pending enrollment.
	if remotePeerID == "" {
		return errors.New("paired Device peer ID is required")
	}
	pendingPeerID, _, err := peer.ParsePeerIDWithPubKey(remotePeerID)
	if err != nil {
		return errors.Wrap(err, "parse paired Device peer id")
	}
	a.markP2PPendingEnrollPeer(pendingPeerID)

	// Mount the settings object and queue the paired-device operations.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return errors.Wrap(err, "mount account settings")
	}
	defer release()
	return a.queueAddPairedDevice(ctx, so, remotePeerID, displayName)
}

// queueAddPairedDevice queues an AddPairedDevice operation on the given
// (already-mounted) shared object.
func (a *ProviderAccount) queueAddPairedDevice(
	ctx context.Context,
	so sobject.SharedObject,
	remotePeerIDStr string,
	displayName string,
) error {
	// Normalize the paired-device display name.
	if displayName == "" {
		displayName = "Device"
	}

	// Build and marshal the paired-device operation.
	addOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_AddPairedDevice{
			AddPairedDevice: &account_settings.PairedDevice{
				PeerId:      remotePeerIDStr,
				DisplayName: displayName,
				PairedAt:    time.Now().Unix(),
			},
		},
	}
	opData, err := addOp.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal add paired device op")
	}

	// Queue the paired-device operation.
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		return errors.Wrap(err, "queue add paired device operation")
	}

	// Build and queue the session-presentation operation.
	presOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_UpsertSessionPresentation{
			UpsertSessionPresentation: &account_settings.SessionPresentation{
				PeerId:     remotePeerIDStr,
				Label:      displayName,
				DeviceType: "linked",
				ClientName: "Linked device",
			},
		},
	}
	presData, err := presOp.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal session presentation op")
	}
	if _, err := so.QueueOperation(ctx, presData); err != nil {
		return errors.Wrap(err, "queue session presentation operation")
	}
	return nil
}

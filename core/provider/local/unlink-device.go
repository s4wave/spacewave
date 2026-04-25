package provider_local

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// UnlinkDevice removes a paired device from the account settings SO and
// revokes its SO participant access on all shared objects.
func (a *ProviderAccount) UnlinkDevice(ctx context.Context, remotePeerID peer.ID) error {
	remotePeerIDStr := remotePeerID.String()
	accountSettingsRef, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	accountSettingsID := accountSettingsRef.GetProviderResourceRef().GetId()

	soList := a.soListCtr.GetValue()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()

		so, relSO, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to mount SO for unlink")
			continue
		}

		if err := a.removeSOParticipant(ctx, so, remotePeerIDStr); err != nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to remove participant from SO")
		}

		if soID == accountSettingsID {
			if err := a.queueRemovePairedDevice(ctx, so, remotePeerIDStr); err != nil {
				relSO()
				return errors.Wrap(err, "remove paired device")
			}
			if err := a.queueRemoveSessionPresentation(ctx, so, remotePeerIDStr); err != nil {
				relSO()
				return errors.Wrap(err, "remove session presentation")
			}
		}

		relSO()
	}

	return nil
}

func (a *ProviderAccount) queueRemoveSessionPresentation(
	ctx context.Context,
	so sobject.SharedObject,
	peerID string,
) error {
	removeOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemoveSessionPresentation{
			RemoveSessionPresentation: &account_settings.RemoveSessionPresentationOp{
				PeerId: peerID,
			},
		},
	}
	opData, err := removeOp.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal remove session presentation op")
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		return errors.Wrap(err, "queue remove session presentation operation")
	}
	return nil
}

// queueRemovePairedDevice queues a RemovePairedDevice operation on the
// given (already-mounted) shared object.
func (a *ProviderAccount) queueRemovePairedDevice(
	ctx context.Context,
	so sobject.SharedObject,
	remotePeerIDStr string,
) error {
	removeOp := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemovePairedDevice{
			RemovePairedDevice: &account_settings.RemovePairedDeviceOp{
				PeerId: remotePeerIDStr,
			},
		},
	}
	opData, err := removeOp.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal remove paired device op")
	}

	if _, err := so.QueueOperation(ctx, opData); err != nil {
		return errors.Wrap(err, "queue remove paired device operation")
	}
	return nil
}

// removeSOParticipant removes a peer's participant config and grant from
// an already-mounted shared object.
func (a *ProviderAccount) removeSOParticipant(
	ctx context.Context,
	so sobject.SharedObject,
	remotePeerIDStr string,
) error {
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("unexpected shared object type")
	}

	volPeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPriv, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume private key")
	}

	_, err = sobject.RemoveSOParticipant(ctx, localSO.soHost, remotePeerIDStr, volPriv, nil)
	return err
}

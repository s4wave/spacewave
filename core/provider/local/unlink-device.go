package provider_local

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// UnlinkDevice removes a paired device from the account settings SO and
// revokes its SO participant access on all shared objects.
func (a *ProviderAccount) UnlinkDevice(ctx context.Context, remotePeerID peer.ID) error {
	// Hold the replica auth lock until unlinking finishes.
	release, err := a.replicaAuth.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Resolve the account settings and the peer identity to revoke.
	remotePeerIDStr := remotePeerID.String()
	accountSettingsRef, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	accountSettingsID := accountSettingsRef.GetProviderResourceRef().GetId()
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}
	identities := []string{remotePeerIDStr}

	// Refuse a peer that is neither a Session nor a paired Device of the account.
	member := settings.FindAccountSession(remotePeerIDStr)
	paired := slices.ContainsFunc(settings.GetPairedDevices(), func(device *account_settings.PairedDevice) bool {
		return device.GetPeerId() == remotePeerIDStr
	})
	if member == nil && !paired {
		return errors.Errorf("peer %s is not a session or paired device of this account", remotePeerIDStr)
	}

	// Revoke the session member and release its replica peer.
	if member != nil {
		writer, err := a.vol.GetPeer(ctx, true)
		if err != nil {
			return err
		}
		if member.GetRevokedByStoragePeerId() != "" && member.GetRevokedByStoragePeerId() != writer.GetPeerID().String() {
			return errors.New("this Session's removal is already being completed by another replica")
		}
		// Commit revocation before removing grants so concurrent discovery fails closed.
		next := member.CloneVT()
		next.Revoked = true
		next.RevokedByStoragePeerId = writer.GetPeerID().String()
		so, releaseSO, err := a.MountSharedObject(ctx, accountSettingsRef, nil)
		if err != nil {
			return err
		}
		err = commitAccountSettingsOp(ctx, so, &account_settings.AccountSettingsOp{
			Op: &account_settings.AccountSettingsOp_UpsertAccountSession{UpsertAccountSession: next},
		})
		releaseSO()
		if err != nil {
			return err
		}
		settings, err = a.readAccountSettings(ctx)
		if err != nil {
			return err
		}
		if accepted := settings.FindAccountSession(remotePeerIDStr); accepted.GetRevokedByStoragePeerId() != next.GetRevokedByStoragePeerId() {
			return errors.New("another replica owns this Session's removal")
		}
		storageShared := false
		for _, other := range settings.GetSessions() {
			if other.GetPeerId() != remotePeerIDStr && !other.GetRevoked() && other.GetStoragePeerId() == member.GetStoragePeerId() {
				storageShared = true
				break
			}
		}
		if !storageShared && member.GetStoragePeerId() != remotePeerIDStr {
			identities = append(identities, member.GetStoragePeerId())
		}
	}
	a.releaseAccountReplicaPeer(remotePeerID)

	// Revoke the peer on every shared object and remove the paired device.
	soList := a.soListCtr.GetValue()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()

		so, relSO, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			return errors.Wrap(err, "mount object for unlink: "+soID)
		}

		for _, identity := range identities {
			if err := a.removeSOParticipant(ctx, so, identity); err != nil {
				relSO()
				return errors.Wrap(err, "revoke object access: "+soID)
			}
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
	// Marshal the remove-session-presentation operation and queue it.
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
	// Marshal the remove-paired-device operation.
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

	// Queue the remove-paired-device operation on the mounted shared object.
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
	// Require the mounted object to be a local shared object.
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("unexpected shared object type")
	}

	// Read the volume peer and its private key for the removal signature.
	volPeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPriv, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume private key")
	}

	// Remove the peer's participant config and grant from the shared object.
	_, err = sobject.RemoveSOParticipant(ctx, localSO.soHost, remotePeerIDStr, volPriv, nil)
	return err
}

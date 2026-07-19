package provider_local

import (
	"context"
	"time"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ConfirmPairingError identifies why a pairing exchange cannot authorize a grant.
type ConfirmPairingError string

const (
	// ErrPairingExchangeMissing means no pairing exchange exists.
	ErrPairingExchangeMissing ConfirmPairingError = "pairing exchange is missing"
	// ErrPairingExchangeUnconfirmed means the pairing exchange has not reached BothConfirmed.
	ErrPairingExchangeUnconfirmed ConfirmPairingError = "pairing exchange is not confirmed"
	// ErrPairingExchangeConsumed means the pairing exchange already authorized a confirmation.
	ErrPairingExchangeConsumed ConfirmPairingError = "pairing exchange is already consumed"
	// ErrPairingExchangePeerMismatch means the confirmed exchange belongs to another peer.
	ErrPairingExchangePeerMismatch ConfirmPairingError = "pairing exchange peer does not match"
)

// Error returns the pairing exchange authorization failure.
func (e ConfirmPairingError) Error() string {
	return string(e)
}

// ConfirmPairing confirms a verified pairing by adding the remote peer as
// OWNER on all SharedObjects in the account, persisting the paired device
// to the account settings SO, and optionally starting P2P sync.
func (a *ProviderAccount) ConfirmPairing(
	ctx context.Context,
	remotePeerID peer.ID,
	displayName string,
) error {
	if err := a.consumePairingExchange(remotePeerID); err != nil {
		return err
	}
	remotePeerIDStr := remotePeerID.String()

	remotePub, err := remotePeerID.ExtractPublicKey()
	if err != nil {
		return errors.Wrap(err, "extract remote public key")
	}

	// Get the volume's peer identity (used for SO participant/grant operations).
	volPeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return errors.Wrap(err, "get volume peer")
	}
	volPeerIDStr := volPeer.GetPeerID().String()
	volPriv, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return errors.Wrap(err, "get volume private key")
	}

	accountSettingsRef, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get account settings ref")
	}
	accountSettingsID := accountSettingsRef.GetProviderResourceRef().GetId()

	// Add remote peer as OWNER on all SOs. For the account settings SO,
	// also queue the AddPairedDevice operation in the same mount.
	soList := a.soListCtr.GetValue()
	for _, entry := range soList.GetSharedObjects() {
		ref := entry.GetRef()
		soID := ref.GetProviderResourceRef().GetId()
		isAccountSettings := soID == accountSettingsID

		so, relSO, err := a.MountSharedObject(ctx, ref, nil)
		if err != nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to mount SO")
			continue
		}

		if err := a.addSOParticipant(ctx, so, soID, volPriv, volPeerIDStr, remotePeerIDStr, remotePub); err != nil {
			a.le.WithError(err).WithField("so-id", soID).Warn("failed to add participant to SO")
		}

		// Persist paired device while the account settings SO is still mounted.
		if isAccountSettings {
			if err := a.queueAddPairedDevice(ctx, so, remotePeerIDStr, displayName); err != nil {
				relSO()
				return errors.Wrap(err, "persist paired device")
			}
		}

		relSO()
	}

	// Start P2P sync if the session transport is running.
	if st := a.GetSessionTransport(); st != nil {
		if err := a.StartP2PSync(ctx, st); err != nil {
			a.le.WithError(err).Warn("failed to start P2P sync after pairing confirmation")
		}
	}

	return nil
}

func (a *ProviderAccount) consumePairingExchange(remotePeerID peer.ID) error {
	var err error
	a.pairingBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		switch {
		case a.pairing == nil:
			err = ErrPairingExchangeMissing
		case a.pairing.remotePeerID != remotePeerID:
			err = ErrPairingExchangePeerMismatch
		case a.pairing.status != PairingStatusBothConfirmed:
			err = ErrPairingExchangeUnconfirmed
		case a.pairing.confirmationConsumed:
			err = ErrPairingExchangeConsumed
		default:
			a.pairing.confirmationConsumed = true
		}
	})
	return err
}

// queueAddPairedDevice queues an AddPairedDevice operation on the given
// (already-mounted) shared object.
func (a *ProviderAccount) queueAddPairedDevice(
	ctx context.Context,
	so sobject.SharedObject,
	remotePeerIDStr string,
	displayName string,
) error {
	if displayName == "" {
		displayName = "Device"
	}

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

	if _, err := so.QueueOperation(ctx, opData); err != nil {
		return errors.Wrap(err, "queue add paired device operation")
	}

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

// addSOParticipant adds a remote peer as OWNER on an already-mounted
// shared object and issues an encrypted grant for the peer.
func (a *ProviderAccount) addSOParticipant(
	ctx context.Context,
	so sobject.SharedObject,
	soID string,
	localPriv crypto.PrivKey,
	localPeerIDStr string,
	remotePeerIDStr string,
	remotePub crypto.PubKey,
) error {
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("unexpected shared object type")
	}
	_, err := sobject.AddSOParticipant(
		ctx,
		localSO.soHost,
		soID,
		localPriv,
		localPeerIDStr,
		remotePeerIDStr,
		remotePub,
		sobject.SOParticipantRole_SOParticipantRole_OWNER,
		"",
	)
	return err
}

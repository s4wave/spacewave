package provider_local

import (
	"context"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	sobject "github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
)

// AutoStartP2PSyncIfPaired starts P2P sync when the account already has
// paired devices recorded in AccountSettings. Called from the session mount
// path so a session that was paired in a prior mount restores its DEX +
// SOSync controllers without requiring the user to re-pair.
//
// When no paired devices are present this is a no-op. Errors mounting the
// account settings SO are logged and swallowed so a missing or unreadable
// SO does not abort the session mount.
func (a *ProviderAccount) AutoStartP2PSyncIfPaired(
	ctx context.Context,
	st *transport.SessionTransport,
) error {
	if st == nil {
		return nil
	}

	devices, err := a.readPairedDevices(ctx)
	if err != nil {
		a.le.WithError(err).Warn("failed to read paired devices for auto-start")
		return nil
	}
	if len(devices) == 0 {
		return nil
	}

	a.le.WithField("paired-device-count", len(devices)).
		Debug("auto-starting P2P sync for paired devices")
	if err := a.StartP2PSync(ctx, st); err != nil {
		return errors.Wrap(err, "auto-start P2P sync")
	}
	return nil
}

// readPairedDevices mounts the account settings SO and returns its current
// paired_devices list. Returns an empty slice when the SO has no state yet.
func (a *ProviderAccount) readPairedDevices(
	ctx context.Context,
) ([]*account_settings.PairedDevice, error) {
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		if err == sobject.ErrSharedObjectNotFound {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get account settings ref")
	}

	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount account settings")
	}
	defer relSO()

	localSO, ok := so.(*SharedObject)
	if !ok {
		return nil, errors.New("unexpected shared object type")
	}

	stateCtr, relStateCtr, err := localSO.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "access account settings state")
	}
	defer relStateCtr()

	snap, err := stateCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "wait for account settings snapshot")
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "decode account settings root inner")
	}
	if rootInner == nil {
		return nil, nil
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, errors.Wrap(err, "unmarshal account settings state")
		}
	}
	return settings.GetPairedDevices(), nil
}

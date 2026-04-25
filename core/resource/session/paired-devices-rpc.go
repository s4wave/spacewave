package resource_session

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchPairedDevices streams the list of paired devices from the account
// settings SharedObject.
func (r *SessionResource) WatchPairedDevices(
	req *s4wave_session.WatchPairedDevicesRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchPairedDevicesStream,
) error {
	ctx, ctxCancel := context.WithCancel(strm.Context())
	defer ctxCancel()

	var soRef *sobject.SharedObjectRef
	var err error
	if localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount); ok && localAcc != nil {
		soRef, err = localAcc.GetAccountSettingsRef(ctx)
		if err != nil {
			return err
		}
	} else {
		return errors.New("paired devices require local provider account")
	}

	// Mount the account settings SO.
	so, mountRef, err := sobject.ExMountSharedObject(ctx, r.b, soRef, false, ctxCancel)
	if err != nil {
		return err
	}
	defer mountRef.Release()

	// Watch state changes and stream paired devices.
	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, ctxCancel)
	if err != nil {
		return err
	}
	defer relStateCtr()

	var prev *s4wave_session.WatchPairedDevicesResponse
	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return nil
			}
			rootInner, err := snap.GetRootInner(ctx)
			if err != nil {
				return err
			}
			settings := &account_settings.AccountSettings{}
			if data := rootInner.GetStateData(); len(data) > 0 {
				if err := settings.UnmarshalVT(data); err != nil {
					return err
				}
			}
			devices := settings.GetPairedDevices()
			var onlinePeerIDs []string
			if localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount); ok && localAcc != nil && len(devices) > 0 {
				peerIDs := make([]string, len(devices))
				for i, d := range devices {
					peerIDs[i] = d.GetPeerId()
				}
				onlinePeerIDs = localAcc.GetOnlinePeerIDs(ctx, peerIDs)
			}
			resp := &s4wave_session.WatchPairedDevicesResponse{
				PairedDevices: devices,
				OnlinePeerIds: onlinePeerIDs,
			}
			if prev != nil && resp.EqualVT(prev) {
				return nil
			}
			prev = resp
			return strm.Send(resp)
		},
		nil,
	)
}

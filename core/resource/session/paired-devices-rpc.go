package resource_session

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
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

	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok || localAcc == nil {
		return errors.New("paired devices require local provider account")
	}

	var soRef *sobject.SharedObjectRef
	var err error
	soRef, err = localAcc.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
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

	stateCh := make(chan sobject.SharedObjectStateSnapshot, 1)
	errCh := make(chan error, 1)
	go watchPairedDeviceState(ctx, stateCtr, stateCh, errCh)

	var snap sobject.SharedObjectStateSnapshot
	var prev *s4wave_session.WatchPairedDevicesResponse
	for {
		resp, waitChs, err := r.buildPairedDevicesResponse(ctx, localAcc, snap)
		if err != nil {
			return err
		}
		if resp != nil && (prev == nil || !resp.EqualVT(prev)) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}
		next, err := waitPairedDevices(ctx, stateCh, errCh, waitChs)
		if err != nil {
			return err
		}
		if next != nil {
			snap = next
		}
	}
}

func (r *SessionResource) buildPairedDevicesResponse(
	ctx context.Context,
	localAcc *provider_local.ProviderAccount,
	snap sobject.SharedObjectStateSnapshot,
) (*s4wave_session.WatchPairedDevicesResponse, []<-chan struct{}, error) {
	if snap == nil {
		return nil, nil, nil
	}
	rootInner, err := snap.GetRootInner(ctx)
	if err != nil {
		return nil, nil, err
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			return nil, nil, err
		}
	}
	devices := settings.GetPairedDevices()
	var onlinePeerIDs []string
	var waitChs []<-chan struct{}
	if len(devices) > 0 {
		peerIDs := make([]string, len(devices))
		for i, d := range devices {
			peerIDs[i] = d.GetPeerId()
		}
		onlinePeerIDs, waitChs = localAcc.GetOnlinePeerIDsWithWait(peerIDs)
	}
	return &s4wave_session.WatchPairedDevicesResponse{
		PairedDevices: devices,
		OnlinePeerIds: onlinePeerIDs,
	}, waitChs, nil
}

func watchPairedDeviceState(
	ctx context.Context,
	stateCtr interface {
		WaitValueChange(context.Context, sobject.SharedObjectStateSnapshot, <-chan error) (sobject.SharedObjectStateSnapshot, error)
	},
	stateCh chan sobject.SharedObjectStateSnapshot,
	errCh chan<- error,
) {
	var current sobject.SharedObjectStateSnapshot
	for {
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			select {
			case errCh <- err:
			case <-ctx.Done():
			}
			return
		}
		current = next
		select {
		case stateCh <- next:
		default:
			select {
			case <-stateCh:
			default:
			}
			select {
			case stateCh <- next:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func waitPairedDevices(
	ctx context.Context,
	stateCh <-chan sobject.SharedObjectStateSnapshot,
	errCh <-chan error,
	waitChs []<-chan struct{},
) (sobject.SharedObjectStateSnapshot, error) {
	var waitDone <-chan error
	for _, ch := range waitChs {
		if ch != nil {
			waitCtx, cancelWait := context.WithCancel(ctx)
			defer cancelWait()
			done := make(chan error, 1)
			go func() {
				done <- broadcast.WaitAny(waitCtx, waitChs...)
			}()
			waitDone = done
			break
		}
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case next := <-stateCh:
		return next, nil
	case err := <-waitDone:
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

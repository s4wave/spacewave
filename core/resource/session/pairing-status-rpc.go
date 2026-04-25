package resource_session

import (
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchPairingStatus streams pairing state changes during a device linking flow.
func (r *SessionResource) WatchPairingStatus(
	req *s4wave_session.WatchPairingStatusRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchPairingStatusStream,
) error {
	ctx := strm.Context()

	// Only the local provider supports pairing status.
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return strm.Send(&s4wave_session.WatchPairingStatusResponse{
			Status: s4wave_session.PairingStatus_PairingStatus_IDLE,
		})
	}

	bcast := localAcc.GetPairingBroadcast()
	var prev *s4wave_session.WatchPairingStatusResponse
	for {
		var ch <-chan struct{}
		var snap provider_local.PairingSnapshot
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			snap = localAcc.GetPairingSnapshot()
		})

		resp := pairingSnapshotToProto(snap)
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// pairingSnapshotToProto converts a local PairingSnapshot to a proto response.
func pairingSnapshotToProto(snap provider_local.PairingSnapshot) *s4wave_session.WatchPairingStatusResponse {
	resp := &s4wave_session.WatchPairingStatusResponse{
		Status:       s4wave_session.PairingStatus(snap.Status),
		Code:         snap.Code,
		Emoji:        snap.Emoji,
		ErrorMessage: snap.ErrMsg,
	}
	if len(snap.RemotePeerID) > 0 {
		resp.RemotePeerId = snap.RemotePeerID.String()
	}
	return resp
}

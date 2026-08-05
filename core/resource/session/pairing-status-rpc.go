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
	// Initialize the pairing watch context.
	ctx := strm.Context()

	// Return idle status for providers without pairing support.
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return strm.Send(&s4wave_session.WatchPairingStatusResponse{
			Status: s4wave_session.PairingStatus_PairingStatus_IDLE,
		})
	}

	// Capture pairing state and its wake channel.
	bcast := localAcc.GetPairingBroadcast()
	var prev *s4wave_session.WatchPairingStatusResponse
	for {
		var ch <-chan struct{}
		var snap provider_local.PairingSnapshot
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			snap = localAcc.GetPairingSnapshot()
		})

		// Emit only changed pairing snapshots.
		resp := pairingSnapshotToProto(snap)
		if prev == nil || !resp.EqualVT(prev) {
			if err := strm.Send(resp); err != nil {
				return err
			}
			prev = resp
		}

		// Wait for pairing changes or cancellation.
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

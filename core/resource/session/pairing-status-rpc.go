package resource_session

import (
	"github.com/s4wave/spacewave/core/pairing"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// WatchPairingStatus streams pairing state changes during a device linking flow.
func (r *SessionResource) WatchPairingStatus(
	req *s4wave_session.WatchPairingStatusRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchPairingStatusStream,
) error {
	engine, err := r.getPairingEngine()
	if err != nil {
		return err
	}
	var previous *s4wave_session.WatchPairingStatusResponse
	return engine.Watch(strm.Context(), func(snapshot pairing.Snapshot) error {
		response := pairingSnapshotToProto(snapshot)
		if previous != nil && response.EqualVT(previous) {
			return nil
		}
		previous = response
		return strm.Send(response)
	})
}

// pairingSnapshotToProto converts a pairing snapshot to a proto response.
func pairingSnapshotToProto(snap pairing.Snapshot) *s4wave_session.WatchPairingStatusResponse {
	resp := &s4wave_session.WatchPairingStatusResponse{
		Status:       s4wave_session.PairingStatus(snap.Status),
		Code:         snap.Code,
		Emoji:        snap.Emoji,
		ErrorMessage: snap.ErrMsg,
		AccountId:    snap.AccountID,
		AccountName:  snap.AccountName,
		Choice:       snap.Choice.CloneVT(),
		Receiving:    snap.Receiving,
		RemoteLabel:  snap.RemoteLabel,
	}
	if len(snap.RemotePeerID) > 0 {
		resp.RemotePeerId = snap.RemotePeerID.String()
	}
	return resp
}

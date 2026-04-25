package account_settings

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
)

// ProcessAccountSettingsOps is a ProcessOpsFunc that applies AccountSettingsOp
// operations to AccountSettings state data.
func ProcessAccountSettingsOps(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	currentStateData []byte,
	ops []*sobject.SOOperationInner,
) (*[]byte, []*sobject.SOOperationResult, error) {
	state := &AccountSettings{}
	if len(currentStateData) > 0 {
		if err := state.UnmarshalVT(currentStateData); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal account settings state")
		}
	}
	initState := state.CloneVT()

	results := make([]*sobject.SOOperationResult, 0, len(ops))
	for _, opInner := range ops {
		peerID, err := opInner.ParsePeerID()
		if err != nil {
			return nil, nil, err
		}
		peerIDStr := peerID.String()

		op := &AccountSettingsOp{}
		if err := op.UnmarshalVT(opInner.GetOpData()); err != nil {
			results = append(results, sobject.BuildSOOperationResult(
				peerIDStr,
				opInner.GetNonce(),
				false,
				&sobject.SOOperationRejectionErrorDetails{
					ErrorMsg: "invalid op data: " + err.Error(),
				},
			))
			continue
		}

		switch body := op.GetOp().(type) {
		case *AccountSettingsOp_UpdateDisplayName:
			state.DisplayName = body.UpdateDisplayName.GetDisplayName()
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_AddPairedDevice:
			dev := body.AddPairedDevice
			if dev.GetPeerId() == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			// Remove existing entry with same peer_id to avoid duplicates.
			state.PairedDevices = slices.DeleteFunc(state.PairedDevices, func(d *PairedDevice) bool {
				return d.GetPeerId() == dev.GetPeerId()
			})
			state.PairedDevices = append(state.PairedDevices, dev)
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_RemovePairedDevice:
			rmID := body.RemovePairedDevice.GetPeerId()
			if rmID == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			state.PairedDevices = slices.DeleteFunc(state.PairedDevices, func(d *PairedDevice) bool {
				return d.GetPeerId() == rmID
			})
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_AddEntityKeypair:
			kp := body.AddEntityKeypair
			if kp.GetPeerId() == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			// Remove existing entry with same peer_id to avoid duplicates.
			state.EntityKeypairs = slices.DeleteFunc(state.EntityKeypairs, func(k *session.EntityKeypair) bool {
				return k.GetPeerId() == kp.GetPeerId()
			})
			state.EntityKeypairs = append(state.EntityKeypairs, kp)
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_RemoveEntityKeypair:
			rmID := body.RemoveEntityKeypair.GetPeerId()
			if rmID == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			if len(state.EntityKeypairs) <= 1 {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "cannot remove the last entity keypair"},
				))
				continue
			}
			state.EntityKeypairs = slices.DeleteFunc(state.EntityKeypairs, func(k *session.EntityKeypair) bool {
				return k.GetPeerId() == rmID
			})
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_UpsertSessionPresentation:
			pres := body.UpsertSessionPresentation
			if pres.GetPeerId() == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			state.SessionPresentations = slices.DeleteFunc(state.SessionPresentations, func(p *SessionPresentation) bool {
				return p.GetPeerId() == pres.GetPeerId()
			})
			state.SessionPresentations = append(state.SessionPresentations, pres)
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_RemoveSessionPresentation:
			rmID := body.RemoveSessionPresentation.GetPeerId()
			if rmID == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "peer_id is required"},
				))
				continue
			}
			state.SessionPresentations = slices.DeleteFunc(state.SessionPresentations, func(p *SessionPresentation) bool {
				return p.GetPeerId() == rmID
			})
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		default:
			results = append(results, sobject.BuildSOOperationResult(
				peerIDStr, opInner.GetNonce(), false,
				&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "unknown op type"},
			))
		}
	}

	// If no state changes, return nil to signal no-op.
	if state.EqualVT(initState) {
		return nil, results, nil
	}

	nextData, err := state.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal account settings state")
	}
	return &nextData, results, nil
}

package account_settings

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_command "github.com/s4wave/spacewave/sdk/command"
)

// ProcessAccountSettingsOps is a ProcessOpsFunc that applies AccountSettingsOp
// operations to AccountSettings state data.
func ProcessAccountSettingsOps(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	currentStateData []byte,
	ops []*sobject.SOOperationInner,
) (*[]byte, []*sobject.SOOperationResult, error) {
	// Decode the current AccountSettings snapshot.
	state := &AccountSettings{}
	if len(currentStateData) > 0 {
		if err := state.UnmarshalVT(currentStateData); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal account settings state")
		}
	}

	// Preserve the initial state for no-op detection.
	initState := state.CloneVT()

	// Initialize operation results.
	results := make([]*sobject.SOOperationResult, 0, len(ops))

	// Decode and apply each submitted operation.
	for _, opInner := range ops {
		peerID, err := opInner.ParsePeerID()
		if err != nil {
			return nil, nil, err
		}
		peerIDStr := peerID.String()

		// Decode the operation payload before dispatch.
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

		// Dispatch the operation body by its concrete variant.
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

			// Replace any paired-device entry with the same peer ID.
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

			// Replace any entity-keypair entry with the same peer ID.
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

		case *AccountSettingsOp_UpsertKeybindingOverride:
			override := body.UpsertKeybindingOverride
			if err := validateKeybindingOverride(override); err != nil {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: err.Error()},
				))
				continue
			}
			if state.KeybindingOverrides == nil {
				state.KeybindingOverrides = &s4wave_command.KeybindingOverrideSet{Version: 1}
			}
			state.KeybindingOverrides.Overrides = slices.DeleteFunc(
				state.KeybindingOverrides.GetOverrides(),
				func(existing *s4wave_command.KeybindingCommandOverride) bool {
					return existing.GetCommandId() == override.GetCommandId()
				},
			)
			state.KeybindingOverrides.Overrides = append(
				state.KeybindingOverrides.GetOverrides(),
				override.CloneVT(),
			)
			if state.KeybindingOverrides.GetVersion() == 0 {
				state.KeybindingOverrides.Version = 1
			}
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_RemoveKeybindingOverride:
			commandID := body.RemoveKeybindingOverride.GetCommandId()
			if commandID == "" {
				results = append(results, sobject.BuildSOOperationResult(
					peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "command_id is required"},
				))
				continue
			}
			if state.KeybindingOverrides != nil {
				state.KeybindingOverrides.Overrides = slices.DeleteFunc(
					state.KeybindingOverrides.GetOverrides(),
					func(existing *s4wave_command.KeybindingCommandOverride) bool {
						return existing.GetCommandId() == commandID
					},
				)
			}
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_ReplaceKeybindingOverrideSet:
			replacement := body.ReplaceKeybindingOverrideSet
			overrideSet := replacement.GetOverrideSet()
			if err := ValidateKeybindingOverrideSet(overrideSet); err != nil {
				results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: err.Error()}))
				continue
			}
			merged, err := mergeKeybindingOverrideSet(
				state.GetKeybindingOverrides(),
				replacement.GetExpectedOverrideSet(),
				overrideSet,
			)
			if err != nil {
				results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), false,
					&sobject.SOOperationRejectionErrorDetails{ErrorMsg: err.Error()}))
				continue
			}
			state.KeybindingOverrides = merged
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		case *AccountSettingsOp_SetKeybindingSettings:
			if state.KeybindingOverrides == nil {
				state.KeybindingOverrides = &s4wave_command.KeybindingOverrideSet{Version: 1}
			}
			state.KeybindingOverrides.Settings = body.SetKeybindingSettings.CloneVT()
			if state.KeybindingOverrides.GetVersion() == 0 {
				state.KeybindingOverrides.Version = 1
			}
			results = append(results, sobject.BuildSOOperationResult(peerIDStr, opInner.GetNonce(), true, nil))

		default:
			results = append(results, sobject.BuildSOOperationResult(
				peerIDStr, opInner.GetNonce(), false,
				&sobject.SOOperationRejectionErrorDetails{ErrorMsg: "unknown op type"},
			))
		}
	}

	// Return without state data when no operation changed the snapshot.
	if state.EqualVT(initState) {
		return nil, results, nil
	}

	// Marshal the changed AccountSettings snapshot.
	nextData, err := state.MarshalVT()
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal account settings state")
	}
	return &nextData, results, nil
}

func mergeKeybindingOverrideSet(
	current, expected, replacement *s4wave_command.KeybindingOverrideSet,
) (*s4wave_command.KeybindingOverrideSet, error) {
	if expected == nil {
		return nil, errors.New("expected account keybinding override set is required")
	}
	if current == nil {
		current = &s4wave_command.KeybindingOverrideSet{Version: 1}
	}
	if current.EqualVT(expected) {
		return replacement.CloneVT(), nil
	}
	if current.EqualVT(replacement) {
		return current.CloneVT(), nil
	}
	if current.GetVersion() != 2 || expected.GetVersion() != 2 || replacement.GetVersion() != 2 {
		return nil, errors.New("account keybinding override set changed")
	}

	merged := current.CloneVT()
	if keybindingPartitionEqual(current.GetWebOverrides(), current.GetWebSettings(), expected.GetWebOverrides(), expected.GetWebSettings()) {
		merged.WebOverrides = cloneKeybindingOverrides(replacement.GetWebOverrides())
		merged.WebSettings = replacement.GetWebSettings().CloneVT()
	} else if !keybindingPartitionEqual(current.GetWebOverrides(), current.GetWebSettings(), replacement.GetWebOverrides(), replacement.GetWebSettings()) &&
		!keybindingPartitionEqual(expected.GetWebOverrides(), expected.GetWebSettings(), replacement.GetWebOverrides(), replacement.GetWebSettings()) {
		return nil, errors.New("account WEB keybinding overrides changed")
	}
	if keybindingPartitionEqual(current.GetTuiOverrides(), current.GetTuiSettings(), expected.GetTuiOverrides(), expected.GetTuiSettings()) {
		merged.TuiOverrides = cloneKeybindingOverrides(replacement.GetTuiOverrides())
		merged.TuiSettings = replacement.GetTuiSettings().CloneVT()
	} else if !keybindingPartitionEqual(current.GetTuiOverrides(), current.GetTuiSettings(), replacement.GetTuiOverrides(), replacement.GetTuiSettings()) &&
		!keybindingPartitionEqual(expected.GetTuiOverrides(), expected.GetTuiSettings(), replacement.GetTuiOverrides(), replacement.GetTuiSettings()) {
		return nil, errors.New("account TUI keybinding overrides changed")
	}
	return merged, nil
}

func keybindingPartitionEqual(
	leftOverrides []*s4wave_command.KeybindingCommandOverride,
	leftSettings *s4wave_command.KeybindingOverrideSettings,
	rightOverrides []*s4wave_command.KeybindingCommandOverride,
	rightSettings *s4wave_command.KeybindingOverrideSettings,
) bool {
	left := &s4wave_command.KeybindingOverrideSet{
		Version:      2,
		WebOverrides: leftOverrides,
		WebSettings:  leftSettings,
	}
	right := &s4wave_command.KeybindingOverrideSet{
		Version:      2,
		WebOverrides: rightOverrides,
		WebSettings:  rightSettings,
	}
	return left.EqualVT(right)
}

func cloneKeybindingOverrides(
	overrides []*s4wave_command.KeybindingCommandOverride,
) []*s4wave_command.KeybindingCommandOverride {
	cloned := make([]*s4wave_command.KeybindingCommandOverride, len(overrides))
	for i, override := range overrides {
		cloned[i] = override.CloneVT()
	}
	return cloned
}

func validateKeybindingOverride(override *s4wave_command.KeybindingCommandOverride) error {
	if override.GetCommandId() == "" {
		return errors.New("command_id is required")
	}
	if slices.Contains(override.GetClearedBindingIds(), "") {
		return errors.New("cleared binding id is required")
	}
	for _, binding := range override.GetBindings() {
		if binding.GetId() == "" {
			return errors.New("binding id is required")
		}
		if binding.GetBinding() == nil {
			return errors.New("binding value is required")
		}
	}
	return nil
}

// ValidateKeybindingOverrideSet validates the complete version 2 account keybinding override set.
func ValidateKeybindingOverrideSet(overrideSet *s4wave_command.KeybindingOverrideSet) error {
	if overrideSet == nil {
		return errors.New("keybinding override set is required")
	}
	if overrideSet.GetVersion() != 2 {
		return errors.New("keybinding override set version must be 2")
	}
	if len(overrideSet.GetOverrides()) != 0 || overrideSet.GetSettings() != nil {
		return errors.New("version 2 keybinding override set contains legacy fields")
	}
	for _, partition := range []struct {
		name      string
		overrides []*s4wave_command.KeybindingCommandOverride
		surface   s4wave_command.CommandSurface
	}{
		{name: "web", overrides: overrideSet.GetWebOverrides(), surface: s4wave_command.CommandSurface_COMMAND_SURFACE_WEB},
		{name: "tui", overrides: overrideSet.GetTuiOverrides(), surface: s4wave_command.CommandSurface_COMMAND_SURFACE_TUI},
	} {
		seen := make(map[string]struct{}, len(partition.overrides))
		for _, override := range partition.overrides {
			if err := validateKeybindingOverride(override); err != nil {
				return err
			}
			if _, ok := seen[override.GetCommandId()]; ok {
				return errors.New("duplicate command_id in " + partition.name + " partition")
			}
			seen[override.GetCommandId()] = struct{}{}
			for _, binding := range override.GetBindings() {
				if binding.GetSurface() != partition.surface {
					return errors.New("binding surface must match " + partition.name + " partition")
				}
			}
		}
	}
	return nil
}

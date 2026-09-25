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

	// Apply each submitted operation and record its result.
	for _, opInner := range ops {
		peerID, err := opInner.ParsePeerID()
		if err != nil {
			return nil, nil, err
		}
		var rejection *sobject.SOOperationRejectionErrorDetails
		if err := state.applyOpData(opInner.GetOpData()); err != nil {
			rejection = &sobject.SOOperationRejectionErrorDetails{ErrorMsg: err.Error()}
		}
		results = append(results, sobject.BuildSOOperationResult(
			peerID.String(),
			opInner.GetNonce(),
			rejection == nil,
			rejection,
		))
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

// applyOpData decodes one AccountSettingsOp and applies it.
func (s *AccountSettings) applyOpData(opData []byte) error {
	op := &AccountSettingsOp{}
	if err := op.UnmarshalVT(opData); err != nil {
		return errors.Wrap(err, "invalid op data")
	}

	switch body := op.GetOp().(type) {
	case *AccountSettingsOp_UpdateDisplayName:
		s.DisplayName = body.UpdateDisplayName.GetDisplayName()
		return nil
	case *AccountSettingsOp_AddPairedDevice:
		return s.addPairedDevice(body.AddPairedDevice)
	case *AccountSettingsOp_RemovePairedDevice:
		return s.removePairedDevice(body.RemovePairedDevice.GetPeerId())
	case *AccountSettingsOp_AddEntityKeypair:
		return s.addEntityKeypair(body.AddEntityKeypair)
	case *AccountSettingsOp_RemoveEntityKeypair:
		return s.removeEntityKeypair(body.RemoveEntityKeypair.GetPeerId())
	case *AccountSettingsOp_UpsertSessionPresentation:
		return s.upsertSessionPresentation(body.UpsertSessionPresentation)
	case *AccountSettingsOp_RemoveSessionPresentation:
		return s.removeSessionPresentation(body.RemoveSessionPresentation.GetPeerId())
	case *AccountSettingsOp_ReplaceKeybindingOverrideSet:
		return s.replaceKeybindingOverrideSet(body.ReplaceKeybindingOverrideSet)
	case *AccountSettingsOp_UpsertAccountSession:
		return s.applyAccountSession(body.UpsertAccountSession)
	case *AccountSettingsOp_UpsertCatalogEntry:
		return s.applyCatalogEntry(body.UpsertCatalogEntry)
	case *AccountSettingsOp_AcceptAccountMigration:
		return s.acceptAccountMigration(body.AcceptAccountMigration)
	case *AccountSettingsOp_CommitAccountTransition:
		return s.commitAccountTransition(body.CommitAccountTransition)
	case *AccountSettingsOp_UpsertStorageBackend:
		return s.upsertStorageBackend(body.UpsertStorageBackend)
	case *AccountSettingsOp_RemoveStorageBackend:
		return s.removeStorageBackend(body.RemoveStorageBackend.GetStorageBackendId())
	case *AccountSettingsOp_SetDefaultStorageBackend:
		return s.setDefaultStorageBackend(body.SetDefaultStorageBackend.GetStorageBackendId())
	case *AccountSettingsOp_SetBlockStorePlacement:
		return s.setBlockStorePlacement(body.SetBlockStorePlacement)
	case *AccountSettingsOp_CompleteStorageRelease:
		return s.completeStorageRelease(body.CompleteStorageRelease)
	default:
		return errors.New("unknown op type")
	}
}

// addPairedDevice adds a paired device, replacing any entry with its peer ID.
func (s *AccountSettings) addPairedDevice(dev *PairedDevice) error {
	if dev.GetPeerId() == "" {
		return errors.New("peer_id is required")
	}
	s.PairedDevices = slices.DeleteFunc(s.PairedDevices, func(d *PairedDevice) bool {
		return d.GetPeerId() == dev.GetPeerId()
	})
	s.PairedDevices = append(s.PairedDevices, dev)
	return nil
}

// removePairedDevice removes the paired device with the peer ID.
func (s *AccountSettings) removePairedDevice(peerID string) error {
	if peerID == "" {
		return errors.New("peer_id is required")
	}
	s.PairedDevices = slices.DeleteFunc(s.PairedDevices, func(d *PairedDevice) bool {
		return d.GetPeerId() == peerID
	})
	return nil
}

// addEntityKeypair adds an entity keypair, replacing any entry with its peer ID.
func (s *AccountSettings) addEntityKeypair(kp *session.EntityKeypair) error {
	if kp.GetPeerId() == "" {
		return errors.New("peer_id is required")
	}
	s.EntityKeypairs = slices.DeleteFunc(s.EntityKeypairs, func(k *session.EntityKeypair) bool {
		return k.GetPeerId() == kp.GetPeerId()
	})
	s.EntityKeypairs = append(s.EntityKeypairs, kp)
	return nil
}

// removeEntityKeypair removes an entity keypair, keeping at least one so the
// account can still authenticate.
func (s *AccountSettings) removeEntityKeypair(peerID string) error {
	if peerID == "" {
		return errors.New("peer_id is required")
	}
	if len(s.EntityKeypairs) <= 1 {
		return errors.New("cannot remove the last entity keypair")
	}
	s.EntityKeypairs = slices.DeleteFunc(s.EntityKeypairs, func(k *session.EntityKeypair) bool {
		return k.GetPeerId() == peerID
	})
	return nil
}

// upsertSessionPresentation adds or replaces a session presentation row.
func (s *AccountSettings) upsertSessionPresentation(pres *SessionPresentation) error {
	if pres.GetPeerId() == "" {
		return errors.New("peer_id is required")
	}
	s.SessionPresentations = slices.DeleteFunc(s.SessionPresentations, func(p *SessionPresentation) bool {
		return p.GetPeerId() == pres.GetPeerId()
	})
	s.SessionPresentations = append(s.SessionPresentations, pres)
	return nil
}

// removeSessionPresentation removes the session presentation for the peer ID.
func (s *AccountSettings) removeSessionPresentation(peerID string) error {
	if peerID == "" {
		return errors.New("peer_id is required")
	}
	s.SessionPresentations = slices.DeleteFunc(s.SessionPresentations, func(p *SessionPresentation) bool {
		return p.GetPeerId() == peerID
	})
	return nil
}

// replaceKeybindingOverrideSet merges a complete keybinding layer replacement,
// rejecting surfaces that changed since the expected snapshot.
func (s *AccountSettings) replaceKeybindingOverrideSet(replacement *ReplaceKeybindingOverrideSetOp) error {
	overrideSet := replacement.GetOverrideSet()
	if err := ValidateKeybindingOverrideSet(overrideSet); err != nil {
		return err
	}
	merged, err := s4wave_command.MergeKeybindingOverrideSet(
		s.GetKeybindingOverrides(),
		replacement.GetExpectedOverrideSet(),
		overrideSet,
	)
	if err != nil {
		return err
	}
	s.KeybindingOverrides = merged
	return nil
}

// validateKeybindingOverride validates one command's keybinding override.
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

// ValidateKeybindingOverrideSet validates the complete account keybinding override set.
func ValidateKeybindingOverrideSet(overrideSet *s4wave_command.KeybindingOverrideSet) error {
	if overrideSet == nil {
		return errors.New("keybinding override set is required")
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

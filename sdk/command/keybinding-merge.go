package s4wave_command

import (
	"errors"
	"slices"
)

var (
	// ErrKeybindingOverrideSetExpected is returned when a replacement omits its base snapshot.
	ErrKeybindingOverrideSetExpected = errors.New("expected keybinding override set is required")
	// ErrKeybindingOverrideSetChanged is returned when a replacement conflicts with persisted state.
	ErrKeybindingOverrideSetChanged = errors.New("keybinding override set changed")
	// ErrKeybindingOverrideSetReplacement is returned for a malformed replacement.
	ErrKeybindingOverrideSetReplacement = errors.New("replacement keybinding override set is invalid")
)

// MergeKeybindingOverrideSet applies a complete replacement while preserving an
// independently changed WEB or TUI partition.
func MergeKeybindingOverrideSet(
	current, expected, replacement *KeybindingOverrideSet,
) (*KeybindingOverrideSet, error) {
	if expected == nil {
		return nil, ErrKeybindingOverrideSetExpected
	}
	if !validReplacementKeybindingOverrideSet(replacement) {
		return nil, ErrKeybindingOverrideSetReplacement
	}
	if current == nil {
		current = &KeybindingOverrideSet{}
	}
	if current.EqualVT(expected) {
		return replacement.CloneVT(), nil
	}
	if current.EqualVT(replacement) {
		return current.CloneVT(), nil
	}

	merged := current.CloneVT()
	if keybindingPartitionEqual(current.GetWebOverrides(), current.GetWebSettings(), expected.GetWebOverrides(), expected.GetWebSettings()) {
		merged.WebOverrides = cloneKeybindingOverrides(replacement.GetWebOverrides())
		merged.WebSettings = replacement.GetWebSettings().CloneVT()
	} else if !keybindingPartitionEqual(current.GetWebOverrides(), current.GetWebSettings(), replacement.GetWebOverrides(), replacement.GetWebSettings()) &&
		!keybindingPartitionEqual(expected.GetWebOverrides(), expected.GetWebSettings(), replacement.GetWebOverrides(), replacement.GetWebSettings()) {
		return nil, ErrKeybindingOverrideSetChanged
	}
	if keybindingPartitionEqual(current.GetTuiOverrides(), current.GetTuiSettings(), expected.GetTuiOverrides(), expected.GetTuiSettings()) {
		merged.TuiOverrides = cloneKeybindingOverrides(replacement.GetTuiOverrides())
		merged.TuiSettings = replacement.GetTuiSettings().CloneVT()
	} else if !keybindingPartitionEqual(current.GetTuiOverrides(), current.GetTuiSettings(), replacement.GetTuiOverrides(), replacement.GetTuiSettings()) &&
		!keybindingPartitionEqual(expected.GetTuiOverrides(), expected.GetTuiSettings(), replacement.GetTuiOverrides(), replacement.GetTuiSettings()) {
		return nil, ErrKeybindingOverrideSetChanged
	}
	return merged, nil
}

func validReplacementKeybindingOverrideSet(value *KeybindingOverrideSet) bool {
	if value == nil {
		return false
	}
	return validKeybindingPartition(value.GetWebOverrides(), CommandSurface_COMMAND_SURFACE_WEB) &&
		validKeybindingPartition(value.GetTuiOverrides(), CommandSurface_COMMAND_SURFACE_TUI)
}

func validKeybindingPartition(overrides []*KeybindingCommandOverride, surface CommandSurface) bool {
	seen := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		commandID := override.GetCommandId()
		if commandID == "" {
			return false
		}
		if _, duplicate := seen[commandID]; duplicate {
			return false
		}
		seen[commandID] = struct{}{}
		for _, binding := range override.GetBindings() {
			if binding.GetId() == "" || binding.GetBinding() == nil || binding.GetSurface() != surface {
				return false
			}
		}
		if slices.Contains(override.GetClearedBindingIds(), "") {
			return false
		}
	}
	return true
}

func keybindingPartitionEqual(
	leftOverrides []*KeybindingCommandOverride,
	leftSettings *KeybindingOverrideSettings,
	rightOverrides []*KeybindingCommandOverride,
	rightSettings *KeybindingOverrideSettings,
) bool {
	left := &KeybindingOverrideSet{WebOverrides: leftOverrides, WebSettings: leftSettings}
	right := &KeybindingOverrideSet{WebOverrides: rightOverrides, WebSettings: rightSettings}
	return left.EqualVT(right)
}

func cloneKeybindingOverrides(overrides []*KeybindingCommandOverride) []*KeybindingCommandOverride {
	cloned := make([]*KeybindingCommandOverride, len(overrides))
	for i, override := range overrides {
		cloned[i] = override.CloneVT()
	}
	return cloned
}

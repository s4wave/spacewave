package s4wave_viewer_native

import (
	"errors"
	"unicode/utf8"
)

const (
	MaxSessionTabs      = 16
	MaxDraftBytes       = 16 * 1024
	MaxTranscriptOffset = 1 << 20
	MaxIdentityBytes    = 1000
)

var ErrInvalidSelectedState = errors.New("nativeviewer: invalid selected state")

// ValidateSelectedState validates the bounded persistence boundary for selected UI state.
func ValidateSelectedState(state *NativeViewerSelectedState) error {
	// Validate collection bounds before checking cross-field references.
	if state == nil || len(state.GetTabLlmSessionObjectKeys()) > MaxSessionTabs || len(state.GetDraftsByLlmSessionObjectKey()) > MaxSessionTabs || len(state.GetViewportsByLlmSessionObjectKey()) > MaxSessionTabs {
		return ErrInvalidSelectedState
	}
	// Build the LlmSession tab identity set used by focus and viewport validation.
	seen := make(map[string]struct{}, len(state.GetTabLlmSessionObjectKeys()))
	for _, tab := range state.GetTabLlmSessionObjectKeys() {
		if !validIdentity(tab) {
			return ErrInvalidSelectedState
		}
		if _, ok := seen[tab]; ok {
			return ErrInvalidSelectedState
		}
		seen[tab] = struct{}{}
	}
	if state.GetFocusedLlmSessionObjectKey() != "" {
		if !validIdentity(state.GetFocusedLlmSessionObjectKey()) {
			return ErrInvalidSelectedState
		}
		if _, ok := seen[state.GetFocusedLlmSessionObjectKey()]; !ok {
			return ErrInvalidSelectedState
		}
	}
	// Validate bounded drafts and viewport offsets against their LlmSession tab identities.
	for key, draft := range state.GetDraftsByLlmSessionObjectKey() {
		if !validIdentity(key) || !utf8.ValidString(draft) || len(draft) > MaxDraftBytes {
			return ErrInvalidSelectedState
		}
	}
	for key, offset := range state.GetViewportsByLlmSessionObjectKey() {
		if !validIdentity(key) || offset > MaxTranscriptOffset {
			return ErrInvalidSelectedState
		}
		if _, ok := seen[key]; !ok {
			return ErrInvalidSelectedState
		}
	}
	if state.GetSelectedView() > 2 || state.GetTheme() > 2 {
		return ErrInvalidSelectedState
	}
	return nil
}

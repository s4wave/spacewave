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

func validStateIdentity(value string) bool {
	return validIdentity(value)
}

// ValidateSelectedState validates the bounded persistence boundary for selected UI state.
func ValidateSelectedState(state *NativeViewerSelectedState) error {
	if state == nil || len(state.GetTabs()) > MaxSessionTabs || len(state.GetDrafts()) > MaxSessionTabs || len(state.GetViewports()) > MaxSessionTabs {
		return ErrInvalidSelectedState
	}
	seen := make(map[string]struct{}, len(state.GetTabs()))
	for _, tab := range state.GetTabs() {
		if !validStateIdentity(tab) {
			return ErrInvalidSelectedState
		}
		if _, ok := seen[tab]; ok {
			return ErrInvalidSelectedState
		}
		seen[tab] = struct{}{}
	}
	if state.GetFocused() != "" {
		if !validStateIdentity(state.GetFocused()) {
			return ErrInvalidSelectedState
		}
		if _, ok := seen[state.GetFocused()]; !ok {
			return ErrInvalidSelectedState
		}
	}
	for key, draft := range state.GetDrafts() {
		if !validStateIdentity(key) || !utf8.ValidString(draft) || len(draft) > MaxDraftBytes {
			return ErrInvalidSelectedState
		}
	}
	for key, offset := range state.GetViewports() {
		if !validStateIdentity(key) || offset > MaxTranscriptOffset {
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

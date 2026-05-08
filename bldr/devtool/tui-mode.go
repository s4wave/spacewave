//go:build !js

package devtool

import (
	"context"
	"os"
)

// DevtoolUIMode describes how devtool command status should be presented.
type DevtoolUIMode int

const (
	// DevtoolUIModePlain uses normal log output only.
	DevtoolUIModePlain DevtoolUIMode = iota
	// DevtoolUIModeTUI uses the interactive terminal UI.
	DevtoolUIModeTUI
)

// String returns the stable display value.
func (m DevtoolUIMode) String() string {
	switch m {
	case DevtoolUIModeTUI:
		return "tui"
	default:
		return "plain"
	}
}

// ResolveUIMode returns the devtool status presentation mode for this command.
func (a *DevtoolArgs) ResolveUIMode() DevtoolUIMode {
	if a.NoTUI {
		return DevtoolUIModePlain
	}
	if a.terminalDetector != nil {
		if a.terminalDetector() {
			return DevtoolUIModeTUI
		}
		return DevtoolUIModePlain
	}
	if devtoolFileIsTerminal(os.Stdin) && devtoolFileIsTerminal(os.Stdout) {
		return DevtoolUIModeTUI
	}
	return DevtoolUIModePlain
}

// ShouldUseTUI reports whether the devtool should start the interactive TUI.
func (a *DevtoolArgs) ShouldUseTUI() bool {
	return a.CurrentUIMode() == DevtoolUIModeTUI
}

// CurrentUIMode returns the UI mode selected for the active status command.
func (a *DevtoolArgs) CurrentUIMode() DevtoolUIMode {
	if a.hasResolvedUIMode {
		return a.resolvedUIMode
	}
	return a.ResolveUIMode()
}

func (a *DevtoolArgs) runStatusCommand(ctx context.Context, execute func(context.Context) error) error {
	mode := a.ResolveUIMode()
	a.resolvedUIMode = mode
	a.hasResolvedUIMode = true
	if a.Logger != nil {
		a.Logger.WithField("ui-mode", mode.String()).Debug("resolved devtool ui mode")
	}
	return execute(ctx)
}

func devtoolFileIsTerminal(f *os.File) bool {
	return devtoolFileDescriptorIsTerminal(f)
}

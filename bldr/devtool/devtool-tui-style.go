//go:build !js

package devtool

import (
	"github.com/charmbracelet/x/ansi"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// tuiStatusKind is the normalized status category used to pick a glyph and color.
// Every concrete state enum in the status package maps onto one of these so the
// dashboard renders one consistent visual language regardless of the source row.
type tuiStatusKind int

const (
	// tuiStatusNeutral is an uncategorized or unknown state.
	tuiStatusNeutral tuiStatusKind = iota
	// tuiStatusActive is work in progress (starting, running, compiling).
	tuiStatusActive
	// tuiStatusReady is completed or available work.
	tuiStatusReady
	// tuiStatusPending is queued or requested work not yet started.
	tuiStatusPending
	// tuiStatusIdle is attached but not currently doing work.
	tuiStatusIdle
	// tuiStatusError is failed or canceled work needing attention.
	tuiStatusError
	// tuiStatusWarn is recoverable attention that is not a hard failure.
	tuiStatusWarn
)

// ANSI SGR codes used by the dashboard. Kept minimal and 8-color safe so the
// dashboard reads on any terminal. Styling is applied to whole pre-sized tokens
// so it never changes a line's visible width.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
)

// glyph returns the single-cell status marker for the kind.
func (k tuiStatusKind) glyph() string {
	switch k {
	case tuiStatusActive:
		return "⟳"
	case tuiStatusReady:
		return "✓"
	case tuiStatusPending:
		return "○"
	case tuiStatusIdle:
		return "·"
	case tuiStatusError:
		return "✗"
	case tuiStatusWarn:
		return "⚠"
	default:
		return "•"
	}
}

// color returns the ANSI color code for the kind, or empty for the default.
func (k tuiStatusKind) color() string {
	switch k {
	case tuiStatusActive:
		return ansiCyan
	case tuiStatusReady:
		return ansiGreen
	case tuiStatusPending:
		return ansiBlue
	case tuiStatusIdle:
		return ansiDim
	case tuiStatusError:
		return ansiRed
	case tuiStatusWarn:
		return ansiYellow
	default:
		return ""
	}
}

// rank orders kinds so the most urgent work sorts first in a target list.
func (k tuiStatusKind) rank() int {
	switch k {
	case tuiStatusError:
		return 0
	case tuiStatusActive:
		return 1
	case tuiStatusPending:
		return 2
	case tuiStatusReady:
		return 3
	case tuiStatusWarn:
		return 4
	case tuiStatusIdle:
		return 5
	default:
		return 6
	}
}

// tuiTheme applies ANSI styling when color is enabled and passes text through
// unchanged otherwise, so the same render path drives both the live terminal
// and plain-text tests and captures.
type tuiTheme struct {
	color bool
}

// paint wraps text in an ANSI code when styling is enabled.
func (t tuiTheme) paint(code, text string) string {
	if !t.color || code == "" || text == "" {
		return text
	}
	return code + text + ansiReset
}

// kind paints text in the kind's color.
func (t tuiTheme) kind(k tuiStatusKind, text string) string {
	return t.paint(k.color(), text)
}

// commandStatusKind maps a command lifecycle state to a status kind.
func commandStatusKind(state devtool_status.BldrDevtoolCommandState) tuiStatusKind {
	switch state {
	case devtool_status.BldrDevtoolCommandStateStarting,
		devtool_status.BldrDevtoolCommandStateRunning:
		return tuiStatusActive
	case devtool_status.BldrDevtoolCommandStateDone:
		return tuiStatusReady
	case devtool_status.BldrDevtoolCommandStateError:
		return tuiStatusError
	case devtool_status.BldrDevtoolCommandStateCanceled:
		return tuiStatusWarn
	default:
		return tuiStatusNeutral
	}
}

// manifestStatusKind maps a manifest fetch or build state to a status kind.
func manifestStatusKind(state devtool_status.BldrDevtoolManifestState) tuiStatusKind {
	switch state {
	case devtool_status.BldrDevtoolManifestStateQueued:
		return tuiStatusPending
	case devtool_status.BldrDevtoolManifestStateRunning:
		return tuiStatusActive
	case devtool_status.BldrDevtoolManifestStateReady:
		return tuiStatusReady
	case devtool_status.BldrDevtoolManifestStateError:
		return tuiStatusError
	case devtool_status.BldrDevtoolManifestStateCanceled:
		return tuiStatusWarn
	default:
		return tuiStatusNeutral
	}
}

// pluginStatusKind maps a plugin state to a status kind.
func pluginStatusKind(state devtool_status.BldrDevtoolPluginState) tuiStatusKind {
	switch state {
	case devtool_status.BldrDevtoolPluginStateRequested:
		return tuiStatusPending
	case devtool_status.BldrDevtoolPluginStateRunning:
		return tuiStatusActive
	case devtool_status.BldrDevtoolPluginStateErrored:
		return tuiStatusError
	default:
		return tuiStatusNeutral
	}
}

// controllerStatusKind maps a controller state to a status kind.
func controllerStatusKind(state devtool_status.BldrDevtoolControllerState) tuiStatusKind {
	switch state {
	case devtool_status.BldrDevtoolControllerStateRequested:
		return tuiStatusPending
	case devtool_status.BldrDevtoolControllerStateRunning:
		return tuiStatusActive
	case devtool_status.BldrDevtoolControllerStateIdle:
		return tuiStatusIdle
	case devtool_status.BldrDevtoolControllerStateError:
		return tuiStatusError
	default:
		return tuiStatusNeutral
	}
}

// attentionStatusKind maps an attention severity to a status kind.
func attentionStatusKind(severity devtool_status.BldrDevtoolAttentionSeverity) tuiStatusKind {
	switch severity {
	case devtool_status.BldrDevtoolAttentionSeverityWarning:
		return tuiStatusWarn
	case devtool_status.BldrDevtoolAttentionSeverityError:
		return tuiStatusError
	default:
		return tuiStatusNeutral
	}
}

// visibleWidth returns the number of terminal cells occupied by value.
func visibleWidth(value string) int {
	return ansi.StringWidth(value)
}

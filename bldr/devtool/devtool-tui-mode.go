//go:build !js

package devtool

import (
	"os"
	"strings"

	"golang.org/x/term"
)

const bldrNoTUIEnv = "BLDR_NO_TUI"

func (a *DevtoolArgs) shouldRunTUI(input, output *os.File) bool {
	if a.NoTUI || noTUIFromEnv() {
		return false
	}
	return input != nil && output != nil &&
		term.IsTerminal(int(input.Fd())) &&
		term.IsTerminal(int(output.Fd()))
}

func noTUIFromEnv() bool {
	value, ok := os.LookupEnv(bldrNoTUIEnv)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

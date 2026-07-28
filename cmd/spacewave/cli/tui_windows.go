//go:build !js && windows

package spacewave_cli

import "github.com/aperturerobotics/cli"

func appendTuiCommand(commands []*cli.Command) []*cli.Command {
	return commands
}

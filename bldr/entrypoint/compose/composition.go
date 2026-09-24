//go:build !js

package compose

import cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"

// Composition is the project-supplied part of a native host process.
type Composition struct {
	// Factories construct the project's controller factories on each host bus.
	Factories []AddFactoriesFunc
	// Commands build the project's CLI commands.
	Commands []cli_entrypoint.BuildCommandsFunc
}

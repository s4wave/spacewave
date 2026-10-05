//go:build !js

package compose

import (
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// Composition is the project-supplied part of a native host process.
type Composition struct {
	// NativeAction replaces the default no-argument distribution launch. It runs
	// before writable bus construction and independently of NativeRunner.
	NativeAction NativeAction
	// Factories construct the project's controller factories on each host bus.
	Factories []AddFactoriesFunc
	// Commands build the project's CLI commands.
	Commands []cli_entrypoint.BuildCommandsFunc
	// Flags are the project's root CLI flags, accepted before any command.
	Flags []cli.Flag
}

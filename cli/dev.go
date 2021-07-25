package cli

import (
	"errors"

	"github.com/urfave/cli"
)

// DevtoolArgs contains common flags for the dev tools.
type DevtoolArgs struct {
	// CodegenDir is a directory to use for code-generation.
	// If empty, a temporary dir will be used.
	CodegenDir string
	// OutputPath is a path to the output
	OutputPath string
	// NoCleanup indicates we should not cleanup after we are done.
	NoCleanup bool
}

// BuildDevtoolCommand returns the devtool sub-command set.
func (a *DevtoolArgs) BuildDevtoolCommand() cli.Command {
	return cli.Command{
		Name:        "bldr",
		Usage:       "bldr devtools",
		Flags:       a.BuildFlags(),
		Subcommands: a.BuildSubCommands(),
	}
}

// BuildFlags attaches the flags to a flag set.
func (a *DevtoolArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:        "codegen-dir",
			Usage:       "path to directory to create/use for codegen, if empty uses tmpdir",
			EnvVar:      "CONTROLLER_BUS_CODEGEN_DIR",
			Value:       a.CodegenDir,
			Destination: &a.CodegenDir,
		},
		cli.StringFlag{
			Name:        "output, o",
			Usage:       "write the output plugin to `PATH` - accepts {buildHash}",
			EnvVar:      "CONTROLLER_BUS_OUTPUT",
			Value:       a.OutputPath,
			Destination: &a.OutputPath,
		},
		cli.BoolFlag{
			Name:        "no-cleanup",
			Usage:       "disable cleaning up the codegen dirs",
			EnvVar:      "CONTROLLER_BUS_NO_CLEANUP",
			Destination: &a.NoCleanup,
		},
	}
}

// BuildSubCommands builds the sub-command set.
func (a *DevtoolArgs) BuildSubCommands() []cli.Command {
	return []cli.Command{
		cli.Command{
			Name:  "compile",
			Usage: "compile packages specified as arguments into a bundle",
			// Action: a.runCompileOnce,
		},
	}
}

// Validate validates the arguments.
func (a *DevtoolArgs) Validate() error {
	if a.OutputPath == "" {
		return errors.New("output path must be set")
	}
	// more?
	return nil
}

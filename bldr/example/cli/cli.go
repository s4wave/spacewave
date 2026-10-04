// Package example_cli provides example CLI commands for the bldr demo.
//
// This package is referenced from bldr.yaml as a cli_pkgs entry in
// the bldr/cli/compiler manifest. The compiler codegen imports
// NewCliCommands to wire these commands into the generated binary.
package example_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
)

// NewCliCommands builds the example CLI commands.
func NewCliCommands(getBus func() cli_entrypoint.CliBus) []*cli.Command {
	return []*cli.Command{
		{
			Name:  "hello",
			Usage: "print a greeting",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "name",
					Usage: "name to greet",
					Value: "bldr",
				},
			},
			Action: func(c *cli.Context) error {
				// Print the hello command's configured greeting.
				name := c.String("name")
				os.Stdout.WriteString("hello, " + name + "!\n")

				// Report the greeting through the initialized CLI bus.
				b := getBus()
				if b != nil {
					b.GetLogger().Infof("greeted %s via CLI", name)
				}
				return nil
			},
		},
		{
			Name:  "status",
			Usage: "show bus status",
			Action: func(c *cli.Context) error {
				// Require the CLI bus before reporting its status.
				b := getBus()
				if b == nil {
					return errors.New("bus not initialized")
				}

				// Report that the bus is running and print its World engine ID.
				b.GetLogger().Info("bus is running")
				os.Stdout.WriteString("world engine: " + b.GetWorldEngineID() + "\n")
				return nil
			},
		},
	}
}

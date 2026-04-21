// Package example_cli provides example CLI commands for the bldr demo.
//
// This package is referenced from bldr.yaml as a cli_pkgs entry in
// the bldr/cli/compiler manifest. The compiler codegen imports
// NewCliCommands to wire these commands into the generated binary.
package example_cli

import (
	"os"

	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
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
				name := c.String("name")
				os.Stdout.WriteString("hello, " + name + "!\n")
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
				b := getBus()
				if b == nil {
					return errors.New("bus not initialized")
				}
				b.GetLogger().Info("bus is running")
				os.Stdout.WriteString("world engine: " + b.GetWorldEngineID() + "\n")
				return nil
			},
		},
	}
}

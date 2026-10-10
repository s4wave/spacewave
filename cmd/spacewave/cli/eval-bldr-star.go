//go:build !js

package spacewave_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
)

// newEvalBldrStarCommand builds the hidden command that evaluates an untrusted
// bldr.star in a child process. It talks to its parent on stdin and stdout and
// never reads the bus, so it does not displace a running serve process.
func newEvalBldrStarCommand() *cli.Command {
	return &cli.Command{
		Name:   bldr_project_starlark.BoundedCommand,
		Usage:  "evaluate a bldr.star for the parent process (run by spacewave)",
		Hidden: true,
		Action: func(c *cli.Context) error {
			return bldr_project_starlark.RunBounded(c.Context, os.Stdin, os.Stdout)
		},
	}
}

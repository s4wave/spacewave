// Command archive-peer serves the blocks of a stopped s4db file to one named
// peer over the block exchange, so that peer's lookups that miss its own
// stores find them. It opens the file read-only, refuses a file another
// process holds, reads each block in its own read-only transaction, and
// refuses every write. It starts no Volume, provider or collector.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main runs the archive peer until it is interrupted.
func main() {
	// Build the CLI app with the archive peer's flags and action.
	args := &Args{}
	app := cli.NewApp()
	app.Name = "archive-peer"
	app.Usage = "serve a stopped s4db file's blocks to one peer"
	app.Flags = args.BuildFlags()
	app.Action = args.Run
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

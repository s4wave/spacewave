package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main writes the native producer configuration and prints its Bldr path.
func main() {
	args := &Args{}
	app := cli.NewApp()
	app.Name = "release-config"
	app.Flags = args.BuildFlags()
	app.Action = args.Run
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

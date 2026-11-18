//go:build !js && !tinygo && !wasip1

// Command spacewave-world-cold-manifest verifies a local cold-pack export.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main checks the local mirror and writes its manifest without bucket access.
func main() {
	// Bind the offline build to the repository's typed argument parser.
	args := &Args{}
	app := cli.NewApp()
	app.Name = "spacewave-world-cold-manifest"
	app.Usage = "verify a local cold-pack export and build its protobuf manifest"
	app.Flags = args.BuildFlags()
	app.Action = args.Run

	// Report validation errors without a partially published manifest.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// Command spacewave-pack-copy copies what a set of roots reaches from one pack
// store to a clean one, within a connection cap and a byte budget.
//
// It reads the source over SFTP and writes packs to an S3 bucket. Everything it
// must remember between runs lives in one scratch file, so a stopped copy
// resumes where its last durable pack left it.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main runs the command the arguments name.
func main() {
	// Describe the commands.
	args := &Args{}
	app := cli.NewApp()
	app.Name = "spacewave-pack-copy"
	app.Usage = "copy what a set of roots reaches into a clean pack store"
	app.Commands = []*cli.Command{
		{
			Name:   "index",
			Usage:  "build the scratch index of both stores, recording its time",
			Flags:  args.storeFlags(),
			Action: args.runIndex,
		},
		{
			Name:   "copy",
			Usage:  "run one pass, writing what no source could serve to the holes list",
			Flags:  append(args.storeFlags(), args.copyFlags()...),
			Action: args.runCopy,
		},
	}

	// Run the one named.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

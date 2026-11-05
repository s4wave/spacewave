// Command spacewave-pack-copy copies what a set of roots reaches from one pack
// store to a clean one, within a connection cap and a byte budget.
//
// It reads the source over SFTP, finding blocks through a copy of the source's
// pack index file, and writes packs to an S3 bucket, keeping their index in a
// pack index file an owner of the clean store opens as it is. The proofs and
// byte count it remembers between runs live in one scratch file, so a stopped
// copy resumes where its last durable pack left it.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main runs the command the arguments name.
func main() {
	// Describe the commands.
	args, sample := &Args{}, &sampleArgs{}
	app := cli.NewApp()
	app.Name = "spacewave-pack-copy"
	app.Usage = "copy what a set of roots reaches into a clean pack store"
	app.Commands = []*cli.Command{
		{
			Name:   "copy",
			Usage:  "run one pass, writing what no source could serve to the holes list",
			Flags:  append(args.storeFlags(), args.copyFlags()...),
			Action: args.runCopy,
		},
		{
			Name:   "sample",
			Usage:  "time block-sized reads at random places in the source's packs, writing nothing",
			Flags:  args.sampleFlags(sample),
			Action: func(*cli.Context) error { return args.runSample(sample) },
		},
	}

	// Run the one named.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

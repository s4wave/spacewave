// Package packcopy_cli is the pack copy command: it copies what a set of roots
// reaches from one pack store to a clean one, within a connection cap and a
// byte budget.
//
// It reads the source over SFTP, finding blocks through a copy of the source's
// pack index file, and writes packs to an S3 bucket, keeping their index in a
// pack index file an owner of the clean store opens as it is. The proofs and
// byte count it remembers between runs live in one scratch file, so a stopped
// copy resumes where its last durable pack left it.
//
// A program that knows a World's types passes them in Hooks, and the copy then
// decodes the blocks whose refs its store never recorded.
package packcopy_cli

import (
	"os"

	"github.com/aperturerobotics/cli"
	packcopy_world "github.com/s4wave/spacewave/db/block/packcopy/world"
	"github.com/s4wave/spacewave/db/bucket"
)

// Hooks are what a program adds to the command.
type Hooks struct {
	// ReadHead reads the World head file --world-head names: the head, then
	// each earlier root it retains. Nil when the program cannot type a World.
	ReadHead func(path string) ([]*bucket.ObjectRef, error)
	// RootTypes maps the World's object types to their root constructors.
	RootTypes packcopy_world.RootTypes
}

// Main runs the command the process arguments name, and exits on failure.
func Main(name string, hooks *Hooks) {
	if err := NewApp(name, hooks).Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// NewApp builds the command.
func NewApp(name string, hooks *Hooks) *cli.App {
	args, sample := &Args{hooks: hooks}, &sampleArgs{}
	app := cli.NewApp()
	app.Name = name
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
	return app
}

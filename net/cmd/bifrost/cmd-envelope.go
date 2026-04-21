package main

import (
	"github.com/aperturerobotics/cli"
	bcli "github.com/s4wave/spacewave/net/cli"
)

// envelopeArgs are the envelope command arguments.
var envelopeArgs bcli.EnvelopeArgs

func init() {
	commands = append(commands, &cli.Command{
		Name:        "envelope",
		Usage:       "seal and unseal secret-sharing envelopes",
		Subcommands: envelopeArgs.BuildCommands(),
	})
}

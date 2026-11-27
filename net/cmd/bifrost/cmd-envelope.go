package main

import (
	"github.com/aperturerobotics/cli"
	bcli "github.com/s4wave/spacewave/net/cli"
)

// buildEnvelopeCommand constructs the envelope command with its own arguments.
func buildEnvelopeCommand() *cli.Command {
	var envelopeArgs bcli.EnvelopeArgs
	return &cli.Command{
		Name:        "envelope",
		Usage:       "seal and unseal secret-sharing envelopes",
		Subcommands: envelopeArgs.BuildCommands(),
	}
}

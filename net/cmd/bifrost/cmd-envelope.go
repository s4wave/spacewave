package main

import (
	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/net/cli/envelope"
)

// buildEnvelopeCommand constructs the envelope command with its own arguments.
func buildEnvelopeCommand() *cli.Command {
	var envelopeArgs envelope.EnvelopeArgs
	return &cli.Command{
		Name:        "envelope",
		Usage:       "seal and unseal secret-sharing envelopes",
		Subcommands: envelopeArgs.BuildCommands(),
	}
}

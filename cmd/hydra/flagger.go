package main

import (
	"github.com/urfave/cli"
)

type flagger interface {
	// BuildCLIFlags builds the cli flags
	BuildCLIFlags() []cli.Flag
}

// buildFlags builds flags using the flaggers.
func buildFlags(flaggers ...flagger) (flags []cli.Flag) {
	for _, flagger := range flaggers {
		flags = append(flags, flagger.BuildCLIFlags()...)
	}
	return
}

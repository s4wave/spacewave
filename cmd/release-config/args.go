package main

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/cmd/internal/releaseconfig"
)

// Args selects the public environment export for a native producer.
type Args struct {
	// Environment is staging or production; empty uses the production defaults.
	Environment string
}

// BuildFlags binds the release workflow's environment to this command.
func (a *Args) BuildFlags() []cli.Flag {
	return []cli.Flag{&cli.StringFlag{
		Name: "environment", EnvVars: []string{"SPACEWAVE_RELEASE_ENV"}, Destination: &a.Environment,
	}}
}

// Run writes the producer configuration and prints its relative Bldr path.
func (a *Args) Run(*cli.Context) error {
	path, err := releaseconfig.Prepare(".", a.Environment)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(path + "\n")
	return err
}

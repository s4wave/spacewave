package cli

import (
	"errors"

	"github.com/urfave/cli/v2"
)

// DevtoolArgs contains common flags for the dev tools.
type DevtoolArgs struct {
	// OutputPath is the path to use for build output.
	OutputPath string
	// ConfigPath is the path to the bldr.yaml config file.
	ConfigPath string
}

// NewDevtoolArgs constructs new default arguments.
func NewDevtoolArgs() *DevtoolArgs {
	a := &DevtoolArgs{}
	a.FillDefaults()
	return a
}

// FillDefaults fills the args defaults.
func (a *DevtoolArgs) FillDefaults() {
	a.OutputPath = "output"
	a.ConfigPath = "bldr.yaml"
}

// BuildDevtoolCommand returns the devtool sub-command set.
func (a *DevtoolArgs) BuildDevtoolCommand() *cli.Command {
	return &cli.Command{
		Name:        "bldr",
		Usage:       "bldr devtools",
		Flags:       a.BuildFlags(),
		Subcommands: a.BuildSubCommands(),
	}
}

// BuildFlags attaches the flags to a flag set.
func (a *DevtoolArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "config, c",
			Usage:       "use the given path for the bldr config",
			EnvVars:     []string{"BLDR_CONFIG"},
			Value:       a.ConfigPath,
			Destination: &a.ConfigPath,
		},
		&cli.StringFlag{
			Name:        "output, o",
			Usage:       "use the given path for build outputs",
			EnvVars:     []string{"BLDR_OUTPUT"},
			Value:       a.OutputPath,
			Destination: &a.OutputPath,
		},
	}
}

// BuildSubCommands builds the sub-command set.
func (a *DevtoolArgs) BuildSubCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:        "start",
			Usage:       "Start a Bldr application in development mode.",
			Subcommands: a.BuildStartCommands(),
		},
	}
}

// BuildStartCommands builds the bldr start sub-commands.
func (a *DevtoolArgs) BuildStartCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:  "web",
			Usage: "Start the application as a web server.",
			Action: func(c *cli.Context) error {
				return a.StartWeb(c.Context)
			},
		},
		{
			Name:  "electron",
			Usage: "Start the application as an electron app.",
			Action: func(c *cli.Context) error {
				return a.StartElectron(c.Context)
			},
		},
	}
}

// Validate validates the arguments.
func (a *DevtoolArgs) Validate() error {
	if a.OutputPath == "" {
		return errors.New("output path must be set")
	}
	// more?
	return nil
}

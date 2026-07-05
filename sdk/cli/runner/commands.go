package runner

import "github.com/aperturerobotics/cli"

// NewCommands builds the shared browser-safe Spacewave CLI command subset.
func NewCommands(config Config) []*cli.Command {
	return []*cli.Command{
		NewStatusCommand(config),
		NewWhoamiCommand(config),
		NewSpaceCommand(config),
	}
}

// NewStatusCommand builds the shared status command.
func NewStatusCommand(config Config) *cli.Command {
	var sessionIdx uint
	config = config.defaults()
	return &cli.Command{
		Name:  "status",
		Usage: "check daemon health and show summary",
		Flags: config.ClientFlags(&sessionIdx),
		Action: func(c *cli.Context) error {
			return RunStatus(config, c, c.String("output"), uint32(sessionIdx))
		},
	}
}

// NewWhoamiCommand builds the shared whoami command.
func NewWhoamiCommand(config Config) *cli.Command {
	var sessionIdx uint
	config = config.defaults()
	return &cli.Command{
		Name:  "whoami",
		Usage: "show current session identity",
		Flags: config.ClientFlags(&sessionIdx),
		Action: func(c *cli.Context) error {
			return RunWhoami(config, c, c.String("output"), uint32(sessionIdx))
		},
	}
}

// NewSpaceCommand builds the shared space command group.
func NewSpaceCommand(config Config) *cli.Command {
	var sessionIdx uint
	config = config.defaults()
	return &cli.Command{
		Name:    "space",
		Aliases: []string{"spaces"},
		Usage:   "manage spaces",
		Flags:   config.ClientFlags(&sessionIdx),
		Subcommands: []*cli.Command{
			NewSpaceListCommand(config, &sessionIdx),
		},
	}
}

// NewSpaceListCommand builds the shared space list subcommand.
func NewSpaceListCommand(config Config, sessionIdx *uint) *cli.Command {
	var watch bool
	config = config.defaults()
	return &cli.Command{
		Name:  "list",
		Usage: "list spaces in the current session",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "watch",
				Aliases:     []string{"w"},
				Usage:       "watch for changes (append mode)",
				EnvVars:     []string{"SPACEWAVE_WATCH"},
				Destination: &watch,
			},
		},
		Action: func(c *cli.Context) error {
			return RunSpaceList(config, c, c.String("output"), uint32(*sessionIdx), watch)
		},
	}
}

package cli

import (
	"errors"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/util/gitroot"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// DevtoolArgs contains common flags for the dev tools.
type DevtoolArgs struct {
	// Logger is the root logger.
	Logger *logrus.Entry
	// OutputPath is the path to use for build output.
	OutputPath string
	// ConfigPath is the path to the bldr.yaml config file.
	ConfigPath string
	// StatePath is the directory to use for working state.
	StatePath string
	// UseGitRoot enables relative paths to the git repo root.
	UseGitRoot bool
}

// NewDevtoolArgs constructs new default arguments.
func NewDevtoolArgs() *DevtoolArgs {
	a := &DevtoolArgs{}
	a.FillDefaults()
	return a
}

// FillDefaults fills the args defaults.
func (a *DevtoolArgs) FillDefaults() {
	if a.Logger == nil {
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		a.Logger = logrus.NewEntry(log)
	}
	a.OutputPath = "output"
	a.ConfigPath = "bldr.yaml"
	a.StatePath = ".bldr/"
	a.UseGitRoot = true
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
		&cli.StringFlag{
			Name:        "state-path",
			Usage:       "directory to use for working state and file checkouts",
			EnvVars:     []string{"BLDR_STATE_PATH"},
			Value:       a.StatePath,
			Destination: &a.StatePath,
		},
		&cli.BoolFlag{
			Name:        "use-git-root",
			Usage:       "enables always executing at the git repo root",
			EnvVars:     []string{"BLDR_USE_GIT_ROOT"},
			Value:       a.UseGitRoot,
			Destination: &a.UseGitRoot,
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
				return a.ExecuteWeb(c.Context, a.Logger)
			},
		},
		{
			Name:  "electron",
			Usage: "Start the application as an electron app.",
			Action: func(c *cli.Context) error {
				return a.ExecuteElectron(c.Context)
			},
		},
	}
}

// Validate validates the arguments.
func (a *DevtoolArgs) Validate() error {
	if a.OutputPath == "" {
		return errors.New("output path must be set")
	}
	if a.StatePath == "" {
		return errors.New("state path must be set")
	}
	// more?
	return nil
}

// FindRepoRoot returns the absolute path to the root dir to use.
func (a *DevtoolArgs) FindRepoRoot() (string, error) {
	// Resolve the Git root, if set.
	if a.UseGitRoot {
		return gitroot.FindRepoRoot()
	}

	// Use the working directory.
	return os.Getwd()
}

// GetStateRoot returns the state directory according to the config.
func (a *DevtoolArgs) GetStateRoot(repoRoot string) string {
	if confStatePath := a.StatePath; confStatePath != "" {
		if path.IsAbs(confStatePath) {
			return confStatePath
		}
		return path.Join(repoRoot, confStatePath)
	}
	return path.Join(repoRoot, ".bldr")
}

// InitRepoRoot finds an initializes the repo root.
func (a *DevtoolArgs) InitRepoRoot() (
	repoRoot, stateRoot string,
	err error,
) {
	repoRoot, err = a.FindRepoRoot()
	if err != nil {
		return
	}

	stateRoot = a.GetStateRoot(repoRoot)
	err = os.MkdirAll(stateRoot, 0755)
	return repoRoot, stateRoot, err
}

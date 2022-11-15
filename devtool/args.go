package devtool

import (
	"errors"
	"os"
	"path"
	"runtime/debug"

	"github.com/aperturerobotics/bldr"
	"github.com/aperturerobotics/bldr/plugin"
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
	// BldrVersion is the version of bldr to require in go.mod
	BldrVersion string
	// BldrVersionSum is the version sum to require in go.sum
	BldrVersionSum string
	// BuildType is the type of build to perform
	// Usually "dev" or "release"
	BuildType string
	// UseGitRoot enables relative paths to the git repo root.
	UseGitRoot bool
	// MinifyEntrypoint configures if we will minify the entrypoint files.
	MinifyEntrypoint bool
	// WebListenAddr is the address to listen for start:web
	WebListenAddr string
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
	a.BuildType = "dev"
	a.UseGitRoot = true
	a.MinifyEntrypoint = true
	a.WebListenAddr = ":8080"

	if buildInfo, ok := debug.ReadBuildInfo(); ok && buildInfo.Main.Version != "(devel)" {
		a.BldrVersion = buildInfo.Main.Version
		a.BldrVersionSum = buildInfo.Main.Sum
	} else {
		a.BldrVersion = "master"
	}
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
			Usage:       "bldr project config yaml",
			EnvVars:     []string{"BLDR_CONFIG"},
			Value:       a.ConfigPath,
			Destination: &a.ConfigPath,
		},
		&cli.StringFlag{
			Name:        "output, o",
			Usage:       "directory for build outputs",
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
			Usage:       "enables detecting project root with git",
			EnvVars:     []string{"BLDR_USE_GIT_ROOT"},
			Value:       a.UseGitRoot,
			Destination: &a.UseGitRoot,
		},
		&cli.BoolFlag{
			Name:        "minify-entrypoint",
			Usage:       "enables minifying the entrypoint js files",
			EnvVars:     []string{"BLDR_MINIFY_ENTRYPOINT"},
			Value:       a.MinifyEntrypoint,
			Destination: &a.MinifyEntrypoint,
		},
		&cli.StringFlag{
			Name:        "build-type",
			Usage:       "build type: dev or release",
			EnvVars:     []string{"BLDR_BUILD_TYPE"},
			Value:       a.BuildType,
			Destination: &a.BuildType,
		},

		&cli.StringFlag{
			Name:        "bldr-version",
			Usage:       "bldr go module version",
			EnvVars:     []string{"BLDR_VERSION"},
			Value:       a.BldrVersion,
			Destination: &a.BldrVersion,
			Hidden:      true,
		},
		&cli.StringFlag{
			Name:        "bldr-version-sum",
			Usage:       "bldr go module sum",
			EnvVars:     []string{"BLDR_VERSION_SUM"},
			Value:       a.BldrVersionSum,
			Destination: &a.BldrVersionSum,
			Hidden:      true,
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
			Flags:       []cli.Flag{},
		},
		{
			Name:  "setup",
			Usage: "checkout the bldr web sources and dependencies",
			Action: func(c *cli.Context) error {
				return a.ExecuteSetup(c.Context)
			},
		},
	}
}

// BuildStartCommands builds the bldr start sub-commands.
func (a *DevtoolArgs) BuildStartCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:  "electron",
			Usage: "Start the application as an electron app.",
			Action: func(c *cli.Context) error {
				return a.ExecuteElectronProject(c.Context)
			},
		},
		{
			Name:  "web",
			Usage: "Start the application as a web server.",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "listen, l",
					Usage:       "address to listen on for dev build",
					EnvVars:     []string{"BLDR_WEB_LISTEN"},
					Destination: &a.WebListenAddr,
					Value:       a.WebListenAddr,
				},
			},
			Action: func(c *cli.Context) error {
				// TODO: use a separate flag for this? we can run wasm in dev mode too.
				if plugin.ToBuildType(a.BuildType).IsDev() {
					return a.ExecuteWebWsProject(c.Context)
				} else {
					return a.ExecuteWebWasmProject(c.Context)
				}
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
	if err == nil {
		licenseFile := path.Join(stateRoot, "LICENSE.bldr")
		licenseBody := "The Bldr sources are covered by this license:\n\n" + bldr.GetLicense()
		err = os.WriteFile(licenseFile, []byte(licenseBody), 0644)
	}
	if err == nil {
		gitIgnoreFile := path.Join(stateRoot, ".gitignore")
		gitIgnoreBody := "*\n!LICENSE.bldr\n!.gitignore\n"
		err = os.WriteFile(gitIgnoreFile, []byte(gitIgnoreBody), 0644)
	}
	return repoRoot, stateRoot, err
}

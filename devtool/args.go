package devtool

import (
	"errors"
	"os"
	"path"
	"runtime/debug"

	"github.com/aperturerobotics/bldr"
	dist_platform "github.com/aperturerobotics/bldr/dist/platform"
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
	// If running "dist" this is forced to "release"
	BuildType string
	// UseGitRoot enables relative paths to the git repo root.
	UseGitRoot bool
	// MinifyEntrypoint configures if we will minify the entrypoint files.
	MinifyEntrypoint bool
	// WebListenAddr is the address to listen for start:web
	WebListenAddr string
	// WebUseWasm runs the entire runtime in the browser with wasm.
	WebUseWasm bool
	// Watch indicates we should watch for changes.
	Watch bool
	// DistPlatformID is the platform identifier to use for distribution.
	DistPlatformID string
	// TargetsCsv is the list of target IDs.
	TargetsCsv string
	// DisableCleanup disables cleaning up the build files.
	DisableCleanup bool
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
	a.WebListenAddr = ":8080"
	a.MinifyEntrypoint = true
	a.Watch = true
	a.DistPlatformID = dist_platform.DistPlatformID_NATIVE

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
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       "bldr project config yaml",
			EnvVars:     []string{"BLDR_CONFIG"},
			Value:       a.ConfigPath,
			Destination: &a.ConfigPath,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "directory for build outputs",
			EnvVars:     []string{"BLDR_OUTPUT"},
			Value:       a.OutputPath,
			Destination: &a.OutputPath,
		},
		&cli.BoolFlag{
			Name:        "watch",
			Aliases:     []string{"w"},
			Usage:       "watch for changes",
			EnvVars:     []string{"BLDR_WATCH"},
			Value:       a.Watch,
			Destination: &a.Watch,
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
		&cli.BoolFlag{
			Name:        "disable-cleanup",
			Usage:       "disables cleaning up intermediate build files",
			EnvVars:     []string{"BLDR_DISABLE_CLEANUP"},
			Value:       a.DisableCleanup,
			Destination: &a.DisableCleanup,
		},
	}
}

// BuildSubCommands builds the sub-command set.
func (a *DevtoolArgs) BuildSubCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:  "setup",
			Usage: "checkout the bldr web sources and dependencies",
			Action: func(c *cli.Context) error {
				return a.ExecuteSetup(c.Context)
			},
		},
		{
			Name:        "start",
			Usage:       "Start a Bldr application in development mode.",
			Subcommands: a.BuildStartCommands(),
			Flags:       []cli.Flag{},
		},
		a.BuildDistCommand(),
		a.BuildReleaseCommand(),
	}
}

// BuildStartCommands builds the bldr start sub-commands.
func (a *DevtoolArgs) BuildStartCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:  "desktop",
			Usage: "Start the application as a native app.",
			Action: func(c *cli.Context) error {
				return a.ExecuteDesktopProject(c.Context)
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
				&cli.BoolFlag{
					Name:        "wasm",
					Usage:       "if set, use WebAssembly to load the runtime in the browser",
					EnvVars:     []string{"BLDR_WEB_WASM"},
					Destination: &a.WebUseWasm,
					Value:       a.WebUseWasm,
				},
			},
			Action: func(c *cli.Context) error {
				if !a.WebUseWasm {
					return a.ExecuteWebWsProject(c.Context)
				} else {
					return a.ExecuteWebWasmProject(c.Context)
				}
			},
		},
	}
}

// BuildDistCommand builds the bldr dist command.
func (a *DevtoolArgs) BuildDistCommand() *cli.Command {
	return &cli.Command{
		Name:  "dist",
		Usage: "Builds a distribution bundle of the application.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "dist-platform-id",
				Usage:       "distribution platform to target with the distribution bundle",
				EnvVars:     []string{"BLDR_DIST_PLATFORM_ID"},
				Value:       a.DistPlatformID,
				Destination: &a.DistPlatformID,
			},
		},
		Action: func(c *cli.Context) error {
			return a.DistProject(c.Context)
		},
	}
}

// BuildReleaseCommand builds the bldr dist command.
func (a *DevtoolArgs) BuildReleaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "release",
		Usage: "Builds a plugin and/or dist release bundle.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "targets",
				Aliases:     []string{"target", "t"},
				Usage:       "comma-separated list of release target(s) to build",
				EnvVars:     []string{"BLDR_RELEASE_TARGET", "BLDR_RELEASE_TARGETS"},
				Value:       a.TargetsCsv,
				Destination: &a.TargetsCsv,
			},
		},
		Action: func(c *cli.Context) error {
			return a.ReleaseProject(c.Context)
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

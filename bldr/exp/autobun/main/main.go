package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the autobun version.
var Version = "dev"

func main() {
	// Build the base logger at info level.
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	// Hold the CLI flag destinations.
	var (
		stateDir   string
		bunVersion string
		verbose    bool
	)

	// Configure the autobun CLI application and its flags.
	app := cli.NewApp()
	app.Name = "autobun"
	app.Usage = "automatically download and run bun"
	app.ArgsUsage = "[--] <bun arguments...>"
	app.Version = Version
	app.HideVersion = true
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "state-dir",
			Aliases:     []string{"s"},
			Usage:       "directory to store downloaded bun binaries",
			EnvVars:     []string{"AUTOBUN_STATE_DIR"},
			Value:       ".bldr/bun",
			Destination: &stateDir,
		},
		&cli.StringFlag{
			Name:        "bun-version",
			Aliases:     []string{"V"},
			Usage:       "bun version to download",
			EnvVars:     []string{"AUTOBUN_BUN_VERSION", "BUN_VERSION"},
			Value:       autobun.DefaultBunVersion,
			Destination: &bunVersion,
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Aliases:     []string{"v"},
			Usage:       "enable verbose logging",
			EnvVars:     []string{"AUTOBUN_VERBOSE"},
			Destination: &verbose,
		},
	}

	// Run bun with the parsed flags and arguments.
	app.Action = func(c *cli.Context) error {
		// Enable debug logging for the verbose flag.
		if verbose {
			log.SetLevel(logrus.DebugLevel)
		}

		// Resolve state directory relative to git root if relative
		resolvedStateDir := stateDir
		if !filepath.IsAbs(stateDir) {
			root, err := gitroot.FindRepoRoot()
			if err == nil {
				resolvedStateDir = filepath.Join(root, stateDir)
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return errors.Wrap(err, "failed to get working directory")
				}
				resolvedStateDir = filepath.Join(cwd, stateDir)
			}
		}

		// Create a context canceled by SIGINT and SIGTERM.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Cancel the context when a termination signal arrives.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		// Pass the remaining args to bun. Bun flags follow a "--" terminator.
		args := c.Args().Slice()

		return autobun.RunBun(ctx, le, resolvedStateDir, bunVersion, args)
	}

	// Run the CLI and map a child exit code or error to the exit status.
	if err := app.Run(os.Args); err != nil {
		if exitCode, ok := childExitCode(err); ok {
			os.Exit(exitCode)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func childExitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

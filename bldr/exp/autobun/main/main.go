package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/gitroot"
	"github.com/sirupsen/logrus"
)

// Version is the autobun version.
var Version = "dev"

func main() {
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	var (
		stateDir   string
		bunVersion string
		verbose    bool
	)

	app := cli.NewApp()
	app.Name = "autobun"
	app.Usage = "automatically download and run bun"
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

	app.Action = func(c *cli.Context) error {
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
					return fmt.Errorf("failed to get working directory: %w", err)
				}
				resolvedStateDir = filepath.Join(cwd, stateDir)
			}
		}

		// Create context with signal handling
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		// Get remaining args to pass to bun
		args := c.Args().Slice()

		return autobun.RunBun(ctx, le, resolvedStateDir, bunVersion, args)
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

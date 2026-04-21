//go:build !js

package cli_entrypoint

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Main boots the CliBus and runs the CLI application.
func Main(
	appName string,
	factories []AddFactoryFunc,
	configSets []BuildConfigSetFunc,
	commandBuilders []BuildCommandsFunc,
) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var dtBus *CliBusImpl
	getBus := func() CliBus { return dtBus }

	var statePath string
	var logLevel string
	var logFiles cli.StringSlice
	var logFileCleanup func()

	app := cli.NewApp()
	app.Name = appName
	app.HideVersion = true
	app.Usage = appName + " CLI"
	envPrefix := strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "state-path",
			Aliases:     []string{"s"},
			Usage:       "state directory path",
			EnvVars:     []string{"BLDR_STATE_PATH"},
			Value:       ".bldr",
			Destination: &statePath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			Usage:       "log level (debug, info, warn, error)",
			EnvVars:     []string{"BLDR_LOG_LEVEL"},
			Value:       "info",
			Destination: &logLevel,
		},
		logfile.BuildLogFileFlag(&logFiles),
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "output format (json, text, yaml)",
			EnvVars: []string{envPrefix + "_OUTPUT"},
			Value:   "text",
		},
		&cli.StringFlag{
			Name:    "color",
			Usage:   "color mode (auto, always, never)",
			EnvVars: []string{envPrefix + "_COLOR"},
			Value:   "auto",
		},
	}

	app.Before = func(c *cli.Context) error {
		log := logrus.New()
		log.SetFormatter(&logrus.TextFormatter{
			DisableColors:    false,
			DisableTimestamp: false,
		})
		lvl, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return err
		}
		log.SetLevel(lvl)
		le := logrus.NewEntry(log)

		// Attach log file hooks if configured.
		if raw := logFiles.Value(); len(raw) != 0 {
			specs, err := logfile.ParseLogFileSpecs(raw, time.Now())
			if err != nil {
				return err
			}
			if len(specs) != 0 {
				cleanup, err := logfile.AttachLogFiles(log, specs)
				if err != nil {
					return err
				}
				logFileCleanup = cleanup
			}
		}

		root := statePath
		if !filepath.IsAbs(root) {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root = filepath.Join(cwd, root)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}

		b, err := BuildCliBus(ctx, le, root)
		if err != nil {
			return err
		}
		dtBus = b

		for _, fn := range factories {
			if fn == nil {
				continue
			}
			for _, factory := range fn(b.GetBus()) {
				b.GetStaticResolver().AddFactory(factory)
			}
		}

		if len(configSets) != 0 {
			var merged []configset.ConfigSet
			for _, fn := range configSets {
				cs, err := fn(ctx, b.GetBus(), le)
				if err != nil {
					b.Release()
					dtBus = nil
					return err
				}
				merged = append(merged, cs...)
			}
			if len(merged) != 0 {
				set := configset.MergeConfigSets(merged...)
				_, ref, err := b.GetBus().AddDirective(
					configset.NewApplyConfigSet(set),
					nil,
				)
				if err != nil {
					b.Release()
					dtBus = nil
					return err
				}
				_ = ref
			}
		}

		return nil
	}

	app.After = func(c *cli.Context) error {
		if dtBus != nil {
			dtBus.Release()
			dtBus = nil
		}
		if logFileCleanup != nil {
			logFileCleanup()
			logFileCleanup = nil
		}
		return nil
	}

	app.Commands = append(app.Commands, &cli.Command{
		Name:  "start",
		Usage: "start the daemon and block until interrupted",
		Action: func(c *cli.Context) error {
			if dtBus == nil {
				return errors.New("bus not initialized")
			}
			dtBus.GetLogger().Info("started, press ctrl+c to stop")
			<-dtBus.GetContext().Done()
			return nil
		},
	})

	for _, builder := range commandBuilders {
		if builder == nil {
			continue
		}
		app.Commands = append(app.Commands, builder(getBus)...)
	}

	if err := app.RunContext(ctx, os.Args); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

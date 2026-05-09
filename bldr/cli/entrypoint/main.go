//go:build !js

package cli_entrypoint

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/sirupsen/logrus"
)

// Main boots the CliBus and runs the CLI application.
func Main(
	appName string,
	projectID string,
	factories []AddFactoryFunc,
	configSets []BuildConfigSetFunc,
	commandBuilders []BuildCommandsFunc,
) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var dtBus *CliBusImpl
	var statePath string
	var logLevel string
	var logFiles cli.StringSlice
	var logFileCleanup func()
	var configSetRefs []directive.Reference
	var busInitErr error
	var busInitOnce sync.Once
	var le *logrus.Entry

	ensureBus := func() error {
		busInitOnce.Do(func() {
			root := statePath
			if !filepath.IsAbs(root) {
				cwd, err := os.Getwd()
				if err != nil {
					busInitErr = err
					return
				}
				root = filepath.Join(cwd, root)
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				busInitErr = err
				return
			}

			b, err := BuildCliBus(ctx, le, root)
			if err != nil {
				busInitErr = err
				return
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

			if len(configSets) == 0 {
				return
			}
			var merged []configset.ConfigSet
			for _, fn := range configSets {
				cs, err := fn(ctx, b.GetBus(), le)
				if err != nil {
					dtBus.Release()
					dtBus = nil
					busInitErr = err
					return
				}
				merged = append(merged, cs...)
			}
			if len(merged) == 0 {
				return
			}
			set := configset.MergeConfigSets(merged...)
			_, ref, err := b.GetBus().AddDirective(
				configset.NewApplyConfigSet(set),
				nil,
			)
			if err != nil {
				dtBus.Release()
				dtBus = nil
				busInitErr = err
				return
			}
			configSetRefs = append(configSetRefs, ref)
		})
		return busInitErr
	}
	getBus := func() CliBus {
		if err := ensureBus(); err != nil {
			return nil
		}
		return dtBus
	}

	app := cli.NewApp()
	app.Name = appName
	app.HideVersion = true
	app.Usage = appName + " CLI"
	envPrefix := strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))
	defaultStatePath := DefaultStatePath(projectID)
	statePathEnvVars := StatePathEnvVars(projectID)
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "state-path",
			Aliases:     []string{"s"},
			Usage:       "state directory path",
			EnvVars:     statePathEnvVars,
			Value:       defaultStatePath,
			Destination: &statePath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			Usage:       "log level (debug, info, warn, error)",
			EnvVars:     []string{storagepath.LogLevelEnvVar(projectID), "BLDR_LOG_LEVEL"},
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
		le = logrus.NewEntry(log)

		// Attach log file hooks from --log-file / BLDR_LOG_FILE when set.
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
				logfile.EnsureLoggerLevel(log, specs)
				logFileCleanup = cleanup
			}
		}

		// Auto-enable a DEBUG-level file hook under <storageRoot>/logs/.
		// EnableAutoDefault no-ops when BLDR_LOG_FILE is set; the
		// logFileCleanup guard covers the case where --log-file was
		// passed on the command line without setting BLDR_LOG_FILE.
		if logFileCleanup == nil {
			if storageRoot, err := storagepath.DetermineStorageRoot(projectID); err == nil {
				cleanup, err := logfile.EnableAutoDefault(
					log,
					storageRoot,
					storagepath.LogRetentionDaysEnvVar(projectID),
					time.Now(),
				)
				if err != nil {
					le.WithError(err).Warn("failed to enable auto-default log file")
				}
				if cleanup != nil {
					logFileCleanup = cleanup
				}
			}
		}

		return nil
	}

	app.After = func(c *cli.Context) error {
		for _, ref := range configSetRefs {
			ref.Release()
		}
		configSetRefs = nil
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

	var runtimeTracePath string
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "start",
		Usage: "start the daemon and block until interrupted",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "trace",
				Usage:       "write a Go runtime trace for the daemon process",
				EnvVars:     []string{envPrefix + "_TRACE"},
				Destination: &runtimeTracePath,
			},
		},
		Action: func(c *cli.Context) error {
			return runWithRuntimeTrace(runtimeTracePath, func() error {
				if err := ensureBus(); err != nil {
					return err
				}
				if dtBus == nil {
					return errors.New("bus not initialized")
				}
				dtBus.GetLogger().Info("started, press ctrl+c to stop")
				<-dtBus.GetContext().Done()
				return nil
			})
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

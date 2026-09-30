//go:build !js

package cli_entrypoint

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	entrypoint_fatal "github.com/s4wave/spacewave/bldr/entrypoint/fatal"
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
	// Run until the process is interrupted.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Declare the CLI state the flag and command closures mutate.
	var dtBus *CliBusImpl
	var statePath string
	var statePathSet bool
	var socketPath string
	var startSocketPath string
	var logLevel string
	var logFiles cli.StringSlice

	// Declare the log file cleanup hook and release it on exit.
	var logFileCleanup func()
	defer func() {
		if logFileCleanup != nil {
			logFileCleanup()
		}
	}()

	// Declare the bus initialization state shared across closures.
	var configSetRefs []directive.Reference
	var busInitErr error
	var busInitOnce sync.Once
	var le *logrus.Entry

	// ensureBus initializes the CliBus once and registers factories and config sets.
	ensureBus := func() error {
		// Initialize the bus state exactly once per process.
		busInitOnce.Do(func() {
			// Resolve the state root for the project.
			root, err := storagepath.ResolveStatePath(projectID, statePath, socketPath, statePathSet)
			if err != nil {
				busInitErr = err
				return
			}

			// Build the CliBus against the resolved state root.
			b, err := BuildCliBus(ctx, le, projectID, root)
			if err != nil {
				busInitErr = err
				return
			}
			dtBus = b

			// Register every discovered controller factory on the bus resolver.
			for _, fn := range factories {
				if fn == nil {
					continue
				}
				for _, factory := range fn(b.GetBus()) {
					b.GetStaticResolver().AddFactory(factory)
				}
			}

			// Merge the config sets produced by every build function.
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

			// Apply the merged config set on the bus and retain its reference.
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

	// getBus returns the initialized CliBus or nil on failure.
	getBus := func() CliBus {
		if err := ensureBus(); err != nil {
			return nil
		}
		return dtBus
	}

	// Configure the CLI application metadata and error handler.
	app := cli.NewApp()
	app.Name = appName
	app.HideVersion = true
	app.Usage = appName + " CLI"
	var terminalCommand string
	app.ExitErrHandler = func(c *cli.Context, err error) {
		if err == nil || c == nil || c.Command == nil || c.Command.Name == "" {
			return
		}
		if terminalCommand == "" || c.Command.Name != appName {
			terminalCommand = c.Command.HelpName
		}
	}

	// Declare the CLI flags for state, socket, logging, and output.
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
			Name:        "socket-path",
			Usage:       "listen on this exact Unix socket path",
			EnvVars:     []string{envPrefix + "_SOCKET_PATH"},
			Destination: &socketPath,
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

	// app.Before initializes logging and attaches log file hooks.
	app.Before = func(c *cli.Context) error {
		// Skip initialization for the version command.
		if c.Command != nil && c.Command.Name == "version" {
			return nil
		}
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

		// Publish the state path first so the log directory follows it.
		statePathSet = c.IsSet("state-path")
		if _, err := storagepath.ResolveStatePath(projectID, statePath, socketPath, statePathSet); err != nil {
			return err
		}

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

	// app.After releases the config set references and the CliBus.
	app.After = func(c *cli.Context) error {
		// Release every applied config set reference.
		for _, ref := range configSetRefs {
			ref.Release()
		}
		configSetRefs = nil
		if dtBus != nil {
			dtBus.Release()
			dtBus = nil
		}
		return nil
	}

	// Register the version command and the daemon start command.
	var runtimeTracePath string
	app.Commands = append(app.Commands, newStandaloneVersionCommand(projectID))
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
			&cli.StringFlag{
				Name:        "socket-path",
				Usage:       "listen on this exact Unix socket path",
				EnvVars:     []string{envPrefix + "_SOCKET_PATH"},
				Destination: &startSocketPath,
			},
		},
		Action: func(c *cli.Context) error {
			// Run the daemon under an optional runtime trace until interrupted.
			return runWithRuntimeTrace(runtimeTracePath, func() error {
				// Apply the start-specific socket path and ensure the bus is up.
				if startSocketPath != "" {
					socketPath = startSocketPath
				}
				if err := ensureBus(); err != nil {
					return err
				}
				if dtBus == nil {
					return errors.New("bus not initialized")
				}
				dtBus.GetLogger().Info("started, press ctrl+c to stop")
				select {
				case <-dtBus.GetContext().Done():
					return nil
				case <-entrypoint_fatal.Chan():
					// A controller reported a condition under which the
					// daemon cannot serve, such as another live daemon
					// owning the front-door socket. Exit with the error
					// instead of blocking as a daemon without its
					// front door.
					return entrypoint_fatal.Err()
				}
			})
		},
	})

	// Append the commands contributed by the linked CLI packages.
	for _, builder := range commandBuilders {
		if builder == nil {
			continue
		}
		app.Commands = append(app.Commands, builder(getBus)...)
	}

	// Run the CLI application and log the terminal command on failure.
	err := app.RunContext(ctx, os.Args)
	if err != nil && le != nil {
		command := terminalCommand
		if command == "" {
			command = appName
		}
		le.WithError(err).Error(command + " stopped")
	}

	// Release the log file hooks and exit with the CLI error code.
	if logFileCleanup != nil {
		logFileCleanup()
		logFileCleanup = nil
	}
	cli.HandleExitCoder(err)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

type standaloneVersionIdentity struct {
	SchemaVersion  int
	ProjectID      string
	EntrypointRole string
	ChannelKey     string
	PlatformID     string
	Manifest       struct {
		ManifestID string
		Rev        uint64
	}
}

func newStandaloneVersionCommand(projectID string) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print entrypoint version and release identity",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "print machine-readable JSON"},
		},
		Action: func(c *cli.Context) error {
			identity := standaloneVersionIdentity{
				SchemaVersion:  1,
				ProjectID:      projectID,
				EntrypointRole: "standalone",
				PlatformID:     "desktop/" + runtime.GOOS + "/" + runtime.GOARCH,
			}
			if c.Bool("json") {
				_, err := c.App.Writer.Write(marshalStandaloneVersionIdentity(identity))
				return err
			}
			_, err := c.App.Writer.Write([]byte(identity.ProjectID + " " + identity.EntrypointRole + "\n"))
			return err
		},
	}
}

func marshalStandaloneVersionIdentity(identity standaloneVersionIdentity) []byte {
	// Marshal the top-level identity fields into a JSON object.
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("schemaVersion", arena.NewNumberInt(identity.SchemaVersion))
	obj.Set("projectId", arena.NewString(identity.ProjectID))
	obj.Set("entrypointRole", arena.NewString(identity.EntrypointRole))
	obj.Set("channelKey", arena.NewString(identity.ChannelKey))
	obj.Set("platformId", arena.NewString(identity.PlatformID))

	// Marshal the nested manifest identity object.
	manifest := arena.NewObject()
	manifest.Set("manifestId", arena.NewString(identity.Manifest.ManifestID))
	manifest.Set("rev", arena.NewNumberString(strconv.FormatUint(identity.Manifest.Rev, 10)))
	obj.Set("manifest", manifest)
	return append(obj.MarshalTo(nil), '\n')
}

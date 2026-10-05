//go:build !js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/cli"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/sirupsen/logrus"
)

// runCliMain runs the native dist CLI surface.
func runCliMain(
	distMeta *bldr_dist.DistMeta,
	logLevel logrus.Level,
	assetsFS fs.FS,
	composition *compose.Composition,
) error {
	// Bind the CLI lifetime to interrupt handling.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Resolve the project identity and state path defaults.
	projectID := distMeta.GetProjectId()
	appName := projectID
	defaultStatePath := cli_entrypoint.DefaultStatePath(projectID)
	statePathEnvVars := cli_entrypoint.StatePathEnvVars(projectID)
	envPrefix := strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))

	// Hold the CLI flag state the bus initialization reads.
	var dtBus *DistBus
	var statePath string
	var statePathSet bool
	var socketPath string
	var logLevelName string
	var logFiles cli.StringSlice
	var logFileCleanup func()

	// Track bus initialization state and the shared logger entry.
	var busInitErr error
	var busInitOnce sync.Once
	var le *logrus.Entry

	// Initialize the dist bus once from the resolved state path and assets.
	// The closure reads the flag variables set by the CLI parser.
	ensureBus := func() error {
		busInitOnce.Do(func() {
			// Resolve the state root from the CLI flags and environment.
			root, err := storagepath.ResolveStatePath(projectID, statePath, socketPath, statePathSet)
			if err != nil {
				busInitErr = err
				return
			}

			// Read the embedded config set, treating a missing file as empty.
			configSetData, err := fs.ReadFile(assetsFS, "config-set.bin")
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				busInitErr = err
				return
			}

			// Unmarshal the config set proto from the asset bytes.
			configSetProto := &configset_proto.ConfigSet{}
			if err := configSetProto.UnmarshalVT(configSetData); err != nil {
				busInitErr = err
				return
			}

			// Build the dist bus with the embedded block store reader.
			distBus, err := BuildDistBus(
				ctx,
				le,
				distMeta,
				root,
				"",
				configSetProto,
				newStaticBlockStoreReaderBuilder(le, assetsFS, false, distMeta.GetDistWorldRef().GetRootRef()),
				composition,
				nil,
			)
			if err != nil {
				busInitErr = err
				return
			}
			dtBus = distBus
		})
		return busInitErr
	}

	// Return the dist bus after initializing it.
	getBus := func() cli_entrypoint.CliBus {
		if err := ensureBus(); err != nil {
			return nil
		}
		return dtBus
	}

	// Configure the CLI application and its flags.
	app := cli.NewApp()
	app.Name = appName
	app.HideVersion = true
	app.Usage = appName + " CLI"
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
			EnvVars:     []string{storagepath.SocketPathEnvVar(projectID)},
			Destination: &socketPath,
		},
		&cli.StringFlag{
			Name:        "log-level",
			Usage:       "log level (debug, info, warn, error)",
			EnvVars:     []string{storagepath.LogLevelEnvVar(projectID), "BLDR_LOG_LEVEL"},
			Value:       logLevel.String(),
			Destination: &logLevelName,
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
	app.Flags = append(app.Flags, composition.Flags...)

	// Prepare logging and state paths before each command.
	app.Before = func(c *cli.Context) error {
		// Skip logger setup for the version command.
		if c.Command != nil && c.Command.Name == "version" {
			return nil
		}

		// Build the logger from the parsed log level.
		log := logrus.New()
		log.SetFormatter(&logrus.TextFormatter{
			DisableColors:    false,
			DisableTimestamp: false,
		})
		lvl, err := logrus.ParseLevel(logLevelName)
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

		// Attach explicitly configured log files.
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
		// EnableAutoDefault no-ops when BLDR_LOG_FILE is set, so the
		// explicit --log-file branch above takes precedence.
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

	// Register the version command and release state after each run.
	app.Commands = append(app.Commands, newDistVersionCommand(distMeta))
	app.After = func(c *cli.Context) error {
		// Release the dist bus and log files after the command completes.
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

	// Register the composition's CLI commands.
	for _, builder := range composition.Commands {
		if builder == nil {
			continue
		}
		app.Commands = append(app.Commands, builder(getBus)...)
	}

	// Run the CLI application.
	return app.RunContext(ctx, os.Args)
}

// distVersionIdentity is the release identity the version command prints.
type distVersionIdentity struct {
	// SchemaVersion is the version of this record's JSON shape.
	SchemaVersion int
	// ProjectID is the bldr project ID.
	ProjectID string
	// EntrypointRole names the kind of distribution entrypoint.
	EntrypointRole string
	// ChannelKey is the release channel.
	ChannelKey string
	// PlatformID is the distribution's platform.
	PlatformID string
	// StartupPlugins are the plugins the distribution starts.
	StartupPlugins []string
	// Manifest identifies the release manifest.
	Manifest distVersionManifestIdentity
}

// distVersionManifestIdentity identifies a distribution's release manifest.
type distVersionManifestIdentity struct {
	// ManifestID is the manifest ID.
	ManifestID string
	// Rev is the manifest revision.
	Rev uint64
}

// newDistVersionCommand builds the version command from the dist metadata.
func newDistVersionCommand(distMeta *bldr_dist.DistMeta) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print entrypoint version and release identity",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "print machine-readable JSON"},
		},
		Action: func(c *cli.Context) error {
			// Assemble the version identity from the dist metadata.
			identity := distVersionIdentity{
				SchemaVersion:  1,
				ProjectID:      distMeta.GetProjectId(),
				EntrypointRole: distMeta.GetEntrypointRole(),
				ChannelKey:     distMeta.GetChannelKey(),
				PlatformID:     distMeta.GetPlatformId(),
				StartupPlugins: append([]string(nil), distMeta.GetStartupPlugins()...),
				Manifest: distVersionManifestIdentity{
					ManifestID: distMeta.GetManifestId(),
					Rev:        distMeta.GetManifestRev(),
				},
			}

			// Write the identity as JSON or plain text.
			if c.Bool("json") {
				_, err := c.App.Writer.Write(marshalDistVersionIdentity(identity))
				return err
			}
			_, err := c.App.Writer.Write([]byte(identity.ProjectID + " " + identity.EntrypointRole + "\n"))
			return err
		},
	}
}

// marshalDistVersionIdentity encodes identity as a JSON line.
func marshalDistVersionIdentity(identity distVersionIdentity) []byte {
	// Encode the scalar identity fields into a JSON object.
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("schemaVersion", arena.NewNumberInt(identity.SchemaVersion))
	obj.Set("projectId", arena.NewString(identity.ProjectID))
	obj.Set("entrypointRole", arena.NewString(identity.EntrypointRole))
	obj.Set("channelKey", arena.NewString(identity.ChannelKey))
	obj.Set("platformId", arena.NewString(identity.PlatformID))

	// Encode the startup plugins and manifest identity.
	plugins := arena.NewArray()
	for idx, plugin := range identity.StartupPlugins {
		plugins.SetArrayItem(idx, arena.NewString(plugin))
	}
	obj.Set("startupPlugins", plugins)
	manifest := arena.NewObject()
	manifest.Set("manifestId", arena.NewString(identity.Manifest.ManifestID))
	manifest.Set("rev", arena.NewNumberString(strconv.FormatUint(identity.Manifest.Rev, 10)))
	obj.Set("manifest", manifest)
	return append(obj.MarshalTo(nil), '\n')
}

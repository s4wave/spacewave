//go:build !js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/sirupsen/logrus"
)

// runCliMain runs the native dist CLI surface.
func runCliMain(
	distMeta *bldr_dist.DistMeta,
	logLevel logrus.Level,
	assetsFS fs.FS,
	commandBuilders []cli_entrypoint.BuildCommandsFunc,
) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	projectID := distMeta.GetProjectId()
	appName := projectID
	defaultStatePath := cli_entrypoint.DefaultStatePath(projectID)
	statePathEnvVars := cli_entrypoint.StatePathEnvVars(projectID)
	envPrefix := strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))

	var dtBus *DistBus
	var statePath string
	var logLevelName string
	var logFiles cli.StringSlice
	var logFileCleanup func()
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

			configSetData, err := fs.ReadFile(assetsFS, "config-set.bin")
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				busInitErr = err
				return
			}

			configSetProto := &configset_proto.ConfigSet{}
			if err := configSetProto.UnmarshalVT(configSetData); err != nil {
				busInitErr = err
				return
			}

			distBus, err := BuildDistBus(
				ctx,
				le,
				distMeta,
				root,
				"",
				configSetProto,
				newStaticBlockStoreReaderBuilder(le, assetsFS, false, distMeta.GetDistWorldRef().GetRootRef()),
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

	getBus := func() cli_entrypoint.CliBus {
		if err := ensureBus(); err != nil {
			return nil
		}
		return dtBus
	}

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

	app.Before = func(c *cli.Context) error {
		if c.Command != nil && c.Command.Name == "version" {
			return nil
		}
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

	app.Commands = append(app.Commands, newDistVersionCommand(distMeta))
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

	for _, builder := range commandBuilders {
		if builder == nil {
			continue
		}
		app.Commands = append(app.Commands, builder(getBus)...)
	}

	return app.RunContext(ctx, os.Args)
}

type distVersionIdentity struct {
	SchemaVersion  int
	ProjectID      string
	EntrypointRole string
	ChannelKey     string
	PlatformID     string
	StartupPlugins []string
	Manifest       distVersionManifestIdentity
}

type distVersionManifestIdentity struct {
	ManifestID string
	Rev        uint64
}

func newDistVersionCommand(distMeta *bldr_dist.DistMeta) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print entrypoint version and release identity",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "print machine-readable JSON"},
		},
		Action: func(c *cli.Context) error {
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
			if c.Bool("json") {
				_, err := c.App.Writer.Write(marshalDistVersionIdentity(identity))
				return err
			}
			_, err := c.App.Writer.Write([]byte(identity.ProjectID + " " + identity.EntrypointRole + "\n"))
			return err
		},
	}
}

func marshalDistVersionIdentity(identity distVersionIdentity) []byte {
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("schemaVersion", arena.NewNumberInt(identity.SchemaVersion))
	obj.Set("projectId", arena.NewString(identity.ProjectID))
	obj.Set("entrypointRole", arena.NewString(identity.EntrypointRole))
	obj.Set("channelKey", arena.NewString(identity.ChannelKey))
	obj.Set("platformId", arena.NewString(identity.PlatformID))
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

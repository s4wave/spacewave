//go:build !js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cli_entrypoint "github.com/aperturerobotics/bldr/cli/entrypoint"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	"github.com/aperturerobotics/bldr/util/logfile"
	"github.com/aperturerobotics/cli"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
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
				newStaticBlockStoreReaderBuilder(le, assetsFS, false),
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
			EnvVars:     []string{"BLDR_LOG_LEVEL"},
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
				logFileCleanup = cleanup
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

	for _, builder := range commandBuilders {
		if builder == nil {
			continue
		}
		app.Commands = append(app.Commands, builder(getBus)...)
	}

	return app.RunContext(ctx, os.Args)
}

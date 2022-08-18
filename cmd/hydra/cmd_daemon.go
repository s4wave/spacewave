package main

import (
	"context"
	"os"

	bcli "github.com/aperturerobotics/bifrost/cli"
	"github.com/aperturerobotics/bifrost/keypem/keyfile"
	floodsub_controller "github.com/aperturerobotics/bifrost/pubsub/floodsub/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/aperturerobotics/entitygraph/entity"
	hcli "github.com/aperturerobotics/hydra/cli"
	"github.com/aperturerobotics/hydra/daemon"
	api_controller "github.com/aperturerobotics/hydra/daemon/api/controller"
	"github.com/aperturerobotics/hydra/daemon/prof"
	egctr "github.com/aperturerobotics/hydra/entitygraph"
	reconciler_example "github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

type hDaemonArgs = hcli.DaemonArgs
type bDaemonArgs = bcli.DaemonArgs

var daemonFlags struct {
	hDaemonArgs
	bDaemonArgs

	WriteConfig  bool
	ConfigPath   string
	PeerPrivPath string
	APIListen    string
	ProfListen   string
}

func init() {
	dflags := append(
		daemonFlags.hDaemonArgs.BuildFlags(),
		&cli.StringFlag{
			Name:        "node-priv",
			Usage:       "path to node private key, will be generated if doesn't exist",
			Destination: &daemonFlags.PeerPrivPath,
			Value:       "daemon_node_priv.pem",
		},
		&cli.StringFlag{
			Name:        "api-listen",
			Usage:       "if set, will listen on address for API connections, ex :5110",
			Destination: &daemonFlags.APIListen,
			Value:       ":5110",
		},
		&cli.StringFlag{
			Name:        "prof-listen",
			Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
			Destination: &daemonFlags.ProfListen,
		},
		&cli.StringFlag{
			Name:        "config, c",
			Usage:       "path to configuration yaml file",
			EnvVars:     []string{"HYDRA_CONFIG"},
			Value:       "hydra_daemon.yaml",
			Destination: &daemonFlags.ConfigPath,
		},
		&cli.BoolFlag{
			Name:        "write-config",
			Usage:       "write the daemon config file on startup",
			EnvVars:     []string{"HYDRA_WRITE_CONFIG"},
			Destination: &daemonFlags.WriteConfig,
		},
	)
	dflags = append(dflags, daemonFlags.bDaemonArgs.BuildFlags()...)
	commands = append(
		commands,
		&cli.Command{
			Name:   "daemon",
			Usage:  "run a hydra daemon",
			Action: runDaemon,
			Flags:  dflags,
		},
	)
}

// runDaemon runs the daemon.
func runDaemon(c *cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Load or create private key.
	peerPriv, err := keyfile.OpenOrWritePrivKey(le, daemonFlags.PeerPrivPath)
	if err != nil {
		return err
	}

	d, err := daemon.NewDaemon(ctx, peerPriv, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct daemon")
	}

	b := d.GetControllerBus()
	sr := d.GetStaticResolver()
	sr.AddFactory(egctr.NewFactory(b))
	sr.AddFactory(reconciler_example.NewFactory(b))

	// Entity graph controller.
	{
		_, egRef, err := b.AddDirective(
			resolver.NewLoadControllerWithConfig(&egc.Config{}),
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "start entity graph controller")
		}
		defer egRef.Release()
	}

	// Entity graph reporter for bifrost
	{
		_, _, err = b.AddDirective(
			resolver.NewLoadControllerWithConfig(&egctr.Config{}),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("entitygraph bifrost reporter running")
			}, nil, nil),
		)
		if err != nil {
			return errors.Wrap(err, "start entitygraph bifrost reporter")
		}
	}

	// TODO: something better than this logger
	{
		le.Debug("constructing entitygraph logger")
		_, _, err = b.AddDirective(
			entitygraph.NewObserveEntityGraph(),
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				ent := val.GetValue().(entity.Entity)
				le.Infof("EntityGraph: value added: %s: %s", ent.GetEntityTypeName(), ent.GetEntityID())
			}, func(val directive.AttachedValue) {
				ent := val.GetValue().(entity.Entity)
				le.Infof("EntityGraph: value removed: %s: %s", ent.GetEntityTypeName(), ent.GetEntityID())
			}, nil),
		)
		if err != nil {
			return errors.Wrap(err, "start entitygraph logger")
		}
	}

	// ConfigSet controller
	_, csRef, err := b.AddDirective(
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "construct configset controller")
	}
	defer csRef.Release()

	// Daemon API
	if daemonFlags.APIListen != "" {
		_, apiRef, err := b.AddDirective(
			resolver.NewLoadControllerWithConfig(&api_controller.Config{
				ListenAddr: daemonFlags.APIListen,
			}),
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "listen on api")
		}
		defer apiRef.Release()
	}

	// Construct config set.
	confSet := configset.ConfigSet{}

	// Load floodsub controller
	confSet["pubsub"] = configset.NewControllerConfig(1, &floodsub_controller.Config{})

	// Load config file
	configLe := le.WithField("config", daemonFlags.ConfigPath)
	if confPath := daemonFlags.ConfigPath; confPath != "" {
		confDat, err := os.ReadFile(confPath)
		if err != nil {
			if os.IsNotExist(err) {
				if daemonFlags.WriteConfig {
					configLe.Info("cannot find config but write-config is set, continuing")
				} else {
					return errors.Wrapf(
						err,
						"cannot find config at %s",
						daemonFlags.ConfigPath,
					)
				}
			} else {
				return errors.Wrap(err, "load config")
			}
		}

		_, err = configset_json.UnmarshalYAML(ctx, b, confDat, confSet, true)
		if err != nil {
			return errors.Wrap(err, "unmarshal config yaml")
		}
	}

	for _, e := range []error{
		daemonFlags.bDaemonArgs.ApplyToConfigSet(confSet, true),
		daemonFlags.hDaemonArgs.ApplyToConfigSet(confSet, true),
	} {
		if e != nil {
			return e
		}
	}

	if daemonFlags.ConfigPath != "" && daemonFlags.WriteConfig {
		confDat, err := configset_json.MarshalYAML(confSet)
		if err != nil {
			return errors.Wrap(err, "marshal config")
		}
		err = os.WriteFile(daemonFlags.ConfigPath, confDat, 0644)
		if err != nil {
			return errors.Wrap(err, "write config file")
		}
	}

	_, bdbRef, err := b.AddDirective(
		configset.NewApplyConfigSet(confSet),
		nil,
	)
	if err != nil {
		return err
	}
	defer bdbRef.Release()

	if daemonFlags.ProfListen != "" {
		go func() {
			_ = prof.ListenProf(le, daemonFlags.ProfListen)
		}()
	}
	_ = d

	<-ctx.Done()
	return nil
}

//go:build !js && !wasip1

package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/pkg/errors"
	hcli "github.com/s4wave/spacewave/db/cli"
	"github.com/s4wave/spacewave/db/daemon"
	"github.com/s4wave/spacewave/db/daemon/prof"
	bcli "github.com/s4wave/spacewave/net/cli"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/sirupsen/logrus"
)

// daemonArgs binds the Hydra daemon's startup flags and service configuration.
type daemonArgs struct {
	// hydra configures the storage controllers.
	hydra hcli.DaemonArgs
	// bifrost configures the network controllers.
	bifrost bcli.DaemonArgs
	// WriteConfig writes the merged configuration at startup.
	WriteConfig bool
	// ConfigPath is the daemon configuration file.
	ConfigPath string
	// PeerPrivPath is the node private key file.
	PeerPrivPath string
	// APIListen is the daemon API listen address.
	APIListen string
	// ProfListen is the debug profiler listen address.
	ProfListen string
}

// BuildCommand binds the daemon command's startup flags and action.
func (a *daemonArgs) BuildCommand() *cli.Command {
	return &cli.Command{
		Name:   "daemon",
		Usage:  "run a hydra daemon",
		Action: a.Run,
		Flags:  a.BuildFlags(),
	}
}

// BuildFlags binds flags for the daemon and its underlying services.
func (a *daemonArgs) BuildFlags() []cli.Flag {
	// Combine the Hydra flags with the daemon command's startup flags.
	dflags := append(
		a.hydra.BuildFlags(),
		&cli.StringFlag{
			Name:        "node-priv",
			Usage:       "path to node private key, will be generated if doesn't exist",
			Destination: &a.PeerPrivPath,
			Value:       "daemon_node_priv.pem",
		},
		&cli.StringFlag{
			Name:        "api-listen",
			Usage:       "if set, will listen on address for API connections, ex :5110",
			Destination: &a.APIListen,
			Value:       ":5110",
		},
		&cli.StringFlag{
			Name:        "prof-listen",
			Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
			Destination: &a.ProfListen,
		},
		&cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       "path to configuration yaml file",
			EnvVars:     []string{"HYDRA_CONFIG"},
			Value:       "hydra_daemon.yaml",
			Destination: &a.ConfigPath,
		},
		&cli.BoolFlag{
			Name:        "write-config",
			Usage:       "write the daemon config file on startup",
			EnvVars:     []string{"HYDRA_WRITE_CONFIG"},
			Destination: &a.WriteConfig,
		},
	)

	// Include the network service flags after the startup flags.
	return append(dflags, a.bifrost.BuildFlags()...)
}

// Run starts the Hydra daemon and waits for its process context to end.
func (a *daemonArgs) Run(_ *cli.Context) error {
	// Prepare the daemon lifecycle context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Load or create the private key.
	peerPriv, err := keyfile.OpenOrWritePrivKey(le, a.PeerPrivPath)
	if err != nil {
		return err
	}

	// Construct the Hydra daemon with its node identity.
	d, err := daemon.NewDaemon(ctx, peerPriv, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct daemon")
	}

	// Prepare the daemon to resolve and apply ConfigSets.
	csRef, err := d.PrepareConfigSets(ctx)
	if err != nil {
		return err
	}
	defer csRef.Release()

	// Start the daemon API.
	if a.APIListen != "" {
		apiRef, err := d.ServeAPI(ctx, a.APIListen)
		if err != nil {
			return err
		}
		defer apiRef.Release()
	}

	// Construct the config set.
	confSet := configset.ConfigSet{}

	// Load the floodsub controller.
	confSet["pubsub"] = configset.NewControllerConfig(1, &floodsub_controller.Config{})

	// Load the config file.
	configLe := le.WithField("config", a.ConfigPath)
	if confPath := a.ConfigPath; confPath != "" {
		confDat, err := os.ReadFile(confPath)
		if err != nil {
			if os.IsNotExist(err) {
				if a.WriteConfig {
					configLe.Info("cannot find config but write-config is set, continuing")
				} else {
					return errors.Wrapf(
						err,
						"cannot find config at %s",
						a.ConfigPath,
					)
				}
			} else {
				return errors.Wrap(err, "load config")
			}
		}

		if err := d.MergeConfigSetYAML(ctx, confDat, confSet); err != nil {
			return err
		}
	}

	// Apply command-line configuration to the daemon config set.
	for _, e := range []error{
		a.bifrost.ApplyToConfigSet(confSet, true),
		a.hydra.ApplyToConfigSet(confSet, true, nil),
	} {
		if e != nil {
			return e
		}
	}

	// Write the resolved config set when requested.
	if a.ConfigPath != "" && a.WriteConfig {
		confDat, err := configset_json.MarshalYAML(confSet)
		if err != nil {
			return errors.Wrap(err, "marshal config")
		}
		err = os.WriteFile(a.ConfigPath, confDat, 0o644)
		if err != nil {
			return errors.Wrap(err, "write config file")
		}
	}

	// Apply the completed config set to the daemon bus.
	configRef, err := d.ApplyConfigSet(ctx, confSet)
	if err != nil {
		return err
	}
	defer configRef.Release()

	// Start the profiler only when a listen address is configured.
	if a.ProfListen != "" {
		go func() {
			_ = prof.ListenProf(le, a.ProfListen)
		}()
	}

	// Keep the daemon command alive until its lifecycle ends.
	<-ctx.Done()
	return nil
}

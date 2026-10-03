//go:build !js && !wasip1

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	hcli "github.com/s4wave/spacewave/db/cli"
	fcli "github.com/s4wave/spacewave/forge/cli"
	forge_core "github.com/s4wave/spacewave/forge/core"
	"github.com/s4wave/spacewave/forge/daemon"
	api_controller "github.com/s4wave/spacewave/forge/daemon/api/controller"
	forge_lib "github.com/s4wave/spacewave/forge/lib/all"
	bcli "github.com/s4wave/spacewave/net/cli"
	daemon_prof "github.com/s4wave/spacewave/net/daemon/prof"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/sirupsen/logrus"
)

type (
	hDaemonArgs = hcli.DaemonArgs
	bDaemonArgs = bcli.DaemonArgs
	fDaemonArgs = fcli.DaemonArgs
)

var daemonFlags struct {
	hDaemonArgs
	bDaemonArgs
	fDaemonArgs

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
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       "path to configuration yaml file",
			EnvVars:     []string{"FORGE_CONFIG"},
			Value:       "forge_daemon.yaml",
			Destination: &daemonFlags.ConfigPath,
		},
		&cli.BoolFlag{
			Name:        "write-config",
			Usage:       "write the daemon config file on startup",
			EnvVars:     []string{"FORGE_WRITE_CONFIG"},
			Destination: &daemonFlags.WriteConfig,
		},
	)
	dflags = append(dflags, daemonFlags.bDaemonArgs.BuildFlags()...)
	dflags = append(dflags, daemonFlags.fDaemonArgs.BuildFlags()...)
	commands = append(
		commands,
		&cli.Command{
			Name:   "daemon",
			Usage:  "run a forge daemon",
			Action: runDaemon,
			Flags:  dflags,
		},
	)
}

// runDaemon runs the daemon.
func runDaemon(c *cli.Context) error {
	// Run until interrupted or terminated so the deferred releases run.
	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer ctxCancel()

	// Prepare the debug logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Load or create the peer private key.
	peerPriv, err := keyfile.OpenOrWritePrivKey(le, daemonFlags.PeerPrivPath)
	if err != nil {
		return err
	}

	// Construct the daemon around the peer key.
	d, err := daemon.NewDaemon(ctx, peerPriv, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct daemon")
	}

	// Register the forge factories on the daemon bus.
	b := d.GetControllerBus()
	sr := d.GetStaticResolver()
	forge_core.AddFactories(b, sr)
	forge_lib.AddFactories(b, sr)

	// Load the ConfigSet controller.
	_, csRef, err := b.AddDirective(
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "construct configset controller")
	}
	defer csRef.Release()

	// Serve the daemon API when a listen address is configured.
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

	// Load the floodsub controller.
	confSet["pubsub"] = configset.NewControllerConfig(1, &floodsub_controller.Config{})

	// Merge the config file into the config set.
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

	// Apply the command line arguments to the config set.
	for _, e := range []error{
		daemonFlags.bDaemonArgs.ApplyToConfigSet(confSet, true),
		daemonFlags.hDaemonArgs.ApplyToConfigSet(confSet, true, nil),
	} {
		if e != nil {
			return e
		}
	}

	// Write the merged config back when requested.
	if daemonFlags.ConfigPath != "" && daemonFlags.WriteConfig {
		confDat, err := configset_json.MarshalYAML(confSet)
		if err != nil {
			return errors.Wrap(err, "marshal config")
		}
		err = os.WriteFile(daemonFlags.ConfigPath, confDat, 0o644)
		if err != nil {
			return errors.Wrap(err, "write config file")
		}
	}

	// Apply the configuration set for the lifetime of the daemon command.
	_, bdbRef, err := b.AddDirective(
		configset.NewApplyConfigSet(confSet),
		nil,
	)
	if err != nil {
		return err
	}
	defer bdbRef.Release()

	// Serve daemon profiling when a profiler address is configured.
	if daemonFlags.ProfListen != "" {
		go func() {
			_ = daemon_prof.ListenProf(le, daemonFlags.ProfListen)
		}()
	}

	// Run until the daemon context ends.
	<-ctx.Done()
	return nil
}

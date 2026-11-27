//go:build !js && !wasip1

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/pkg/errors"
	hcli "github.com/s4wave/spacewave/db/cli"
	fcli "github.com/s4wave/spacewave/forge/cli"
	forge_daemon "github.com/s4wave/spacewave/forge/daemon"
	"github.com/s4wave/spacewave/net/cli/daemon"
	daemon_prof "github.com/s4wave/spacewave/net/daemon/prof"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
	"github.com/sirupsen/logrus"
)

// daemonArgs holds the flags and process policy for the Forge daemon command.
type daemonArgs struct {
	// hydra holds the Hydra daemon flags.
	hydra hcli.DaemonArgs
	// bifrost holds the Bifrost daemon flags.
	bifrost daemon.DaemonArgs
	// forge holds the common Forge daemon flags.
	forge fcli.DaemonArgs
	// WriteConfig requests writing the merged configuration.
	WriteConfig bool
	// ConfigPath is the configuration file path.
	ConfigPath string
	// PeerPrivPath is the path to the daemon peer's private key.
	PeerPrivPath string
	// APIListen is the optional address of the daemon API.
	APIListen string
	// ProfListen is the optional address of the debug profiler.
	ProfListen string
}

// BuildCommand binds the daemon command's startup flags and action.
func (a *daemonArgs) BuildCommand() *cli.Command {
	return &cli.Command{
		Name:   "daemon",
		Usage:  "run a forge daemon",
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
			EnvVars:     []string{"FORGE_CONFIG"},
			Value:       "forge_daemon.yaml",
			Destination: &a.ConfigPath,
		},
		&cli.BoolFlag{
			Name:        "write-config",
			Usage:       "write the daemon config file on startup",
			EnvVars:     []string{"FORGE_WRITE_CONFIG"},
			Destination: &a.WriteConfig,
		},
	)

	// Include the network and Forge service flags after the startup flags.
	dflags = append(dflags, a.bifrost.BuildFlags()...)
	return append(dflags, a.forge.BuildFlags()...)
}

// Run starts the Forge daemon and waits for its process context to end.
func (a *daemonArgs) Run(_ *cli.Context) error {
	// Run until interrupted or terminated so the deferred releases run.
	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer ctxCancel()

	// Prepare the debug logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Load or create the peer private key.
	peerPriv, err := keyfile.OpenOrWritePrivKey(le, a.PeerPrivPath)
	if err != nil {
		return err
	}

	// Construct the daemon around the peer key.
	d, err := forge_daemon.NewDaemon(ctx, peerPriv, forge_daemon.ConstructOpts{
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

	// Serve the daemon API when a listen address is configured.
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

	// Merge the config file into the config set.
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

	// Apply the command line arguments to the config set.
	for _, e := range []error{
		a.bifrost.ApplyToConfigSet(confSet, true),
		a.hydra.ApplyToConfigSet(confSet, true, nil),
	} {
		if e != nil {
			return e
		}
	}

	// Write the merged config back when requested.
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

	// Apply the configuration set for the lifetime of the daemon command.
	configRef, err := d.ApplyConfigSet(ctx, confSet)
	if err != nil {
		return err
	}
	defer configRef.Release()

	// Serve daemon profiling when a profiler address is configured.
	if a.ProfListen != "" {
		go func() {
			_ = daemon_prof.ListenProf(le, a.ProfListen)
		}()
	}

	// Run until the daemon context ends.
	<-ctx.Done()
	return nil
}

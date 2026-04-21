//go:build !js

package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/cli"
	bus_api "github.com/aperturerobotics/controllerbus/bus/api"
	configset "github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	bcli "github.com/s4wave/spacewave/net/cli"
	"github.com/s4wave/spacewave/net/daemon"
	bifrost_api "github.com/s4wave/spacewave/net/daemon/api"
	api_controller "github.com/s4wave/spacewave/net/daemon/api/controller"
	daemon_prof "github.com/s4wave/spacewave/net/daemon/prof"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	"github.com/sirupsen/logrus"
)

var daemonFlags struct {
	bcli.DaemonArgs

	WriteConfig  bool
	ConfigPath   string
	PeerPrivPath string
	APIListen    string
	ProfListen   string
}

func init() {
	commands = append(
		commands,
		&cli.Command{
			Name:   "daemon",
			Usage:  "run a bifrost daemon",
			Action: runDaemon,
			Flags: append(
				(&daemonFlags.DaemonArgs).BuildFlags(),
				&cli.StringFlag{
					Name:        "config",
					Aliases:     []string{"c"},
					Usage:       "path to configuration yaml file",
					EnvVars:     []string{"BIFROST_CONFIG"},
					Value:       "bifrost_daemon.yaml",
					Destination: &daemonFlags.ConfigPath,
				},
				&cli.BoolFlag{
					Name:        "write-config",
					Usage:       "write the daemon config file on startup",
					EnvVars:     []string{"BIFROST_WRITE_CONFIG"},
					Destination: &daemonFlags.WriteConfig,
				},
				&cli.StringFlag{
					Name:        "node-priv",
					Usage:       "path to node private key, will be generated if doesn't exist",
					EnvVars:     []string{"BIFROST_NODE_PRIV"},
					Value:       "bifrost_daemon.pem",
					Destination: &daemonFlags.PeerPrivPath,
				},
				&cli.StringFlag{
					Name:        "api-listen",
					Usage:       "if set, will listen on address for API srpc connections, ex :5110",
					EnvVars:     []string{"BIFROST_API_LISTEN"},
					Destination: &daemonFlags.APIListen,
				},
				&cli.StringFlag{
					Name:        "prof-listen",
					Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
					EnvVars:     []string{"BIFROST_PROF_LISTEN"},
					Destination: &daemonFlags.ProfListen,
				},
			),
		},
	)
}

// runDaemon runs the daemon.
func runDaemon(c *cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Load private key.
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

	// Construct config set.
	confSet := configset.ConfigSet{}

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

	// Apply factories
	daemonFlags.ApplyFactories(b, sr)

	// Daemon API
	if daemonFlags.APIListen != "" {
		_, _, apiRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(&api_controller.Config{
				ListenAddr: daemonFlags.APIListen,
				ApiConfig:  &bifrost_api.Config{},
				BusApiConfig: &bus_api.Config{
					EnableExecController: true,
				},
			}),
			nil,
		)
		if err != nil {
			return errors.Wrap(err, "listen on api")
		}
		defer apiRef.Release()
		le.Infof("api listening on: %s", daemonFlags.APIListen)
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

	// Load config sets and factories
	if err := daemonFlags.ApplyToConfigSet(confSet, true); err != nil {
		return err
	}

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
			_ = daemon_prof.ListenProf(le, daemonFlags.ProfListen)
		}()
	}

	_ = d
	<-ctx.Done()
	return nil
}

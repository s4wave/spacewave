package main

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/entitygraph"
	egc "github.com/aperturerobotics/entitygraph/controller"
	"github.com/aperturerobotics/entitygraph/entity"
	"github.com/aperturerobotics/hydra/daemon"
	"github.com/aperturerobotics/hydra/daemon/api/controller"
	egctr "github.com/aperturerobotics/hydra/entitygraph"
	"github.com/aperturerobotics/hydra/reconciler/example"
	"github.com/aperturerobotics/hydra/volume/badger"
	"github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/libp2p/go-libp2p-crypto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

// _ enables the profiling endpoints
import _ "net/http/pprof"

var daemonFlags struct {
	PeerPrivPath string
	APIListen    string
	ProfListen   string

	// BadgerDBs contains a list of badger db paths
	// use a YAML configuration file if you want to adjust options.
	BadgerDBs      cli.StringSlice
	InmemDB        bool
	InmemDBVerbose bool
}

func init() {
	commands = append(
		commands,
		cli.Command{
			Name:   "daemon",
			Usage:  "run a hydra daemon",
			Action: runDaemon,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "node-priv",
					Usage:       "path to node private key, will be generated if doesn't exist",
					Destination: &daemonFlags.PeerPrivPath,
					Value:       "daemon_node_priv.pem",
				},
				cli.StringFlag{
					Name:        "api-listen",
					Usage:       "if set, will listen on address for API grpc connections, ex :5110",
					Destination: &daemonFlags.APIListen,
					Value:       ":5110",
				},
				cli.StringFlag{
					Name:        "prof-listen",
					Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
					Destination: &daemonFlags.ProfListen,
				},

				// TODO: YAML configuration
				cli.StringSliceFlag{
					Name:  "badger-db",
					Usage: "set a path to a badger db to load on startup",
					Value: &daemonFlags.BadgerDBs,
				},
				cli.BoolFlag{
					Name:        "inmem-db",
					Usage:       "if set, start a in-memory volume on startup",
					Destination: &daemonFlags.InmemDB,
				},
				cli.BoolFlag{
					Name:        "inmem-db-verbose",
					Usage:       "if set, mark inmem database as verbose. implies --inmem-db",
					Destination: &daemonFlags.InmemDBVerbose,
				},
			},
		},
	)
}

// runDaemon runs the daemon.
func runDaemon(c *cli.Context) error {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	grpc.EnableTracing = daemonFlags.ProfListen != ""

	// Load private key.
	var peerPriv crypto.PrivKey
	peerPrivDat, err := ioutil.ReadFile(daemonFlags.PeerPrivPath)
	if err != nil {
		if os.IsNotExist(err) {
			le.Debug("generating daemon node private key")
			peerPriv, _, err = keypem.GeneratePrivKey()
			if err != nil {
				return errors.Wrap(err, "generate priv key")
			}
		} else {
			return errors.Wrap(err, "read priv key")
		}

		peerPrivDat, err = keypem.MarshalPrivKeyPem(peerPriv)
		if err != nil {
			return errors.Wrap(err, "marshal priv key")
		}

		if err := ioutil.WriteFile(daemonFlags.PeerPrivPath, peerPrivDat, 0644); err != nil {
			return errors.Wrap(err, "write priv key")
		}
	} else {
		peerPriv, err = keypem.ParsePrivKeyPem(peerPrivDat)
		if err != nil {
			return errors.Wrap(err, "parse node priv key")
		}
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
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Info("entity graph controller running")
			}, nil, nil),
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
			bus.NewCallbackHandler(func(val directive.AttachedValue) {
				le.Infof("grpc api listening on: %s", daemonFlags.APIListen)
			}, nil, nil),
		)
		if err != nil {
			return errors.Wrap(err, "listen on grpc api")
		}
		defer apiRef.Release()
	}

	// Construct config set.
	confSet := configset.ConfigSet{}

	// Load defined inmem database
	if daemonFlags.InmemDB || daemonFlags.InmemDBVerbose {
		id := "cli-inmem-volume-0"
		conf := &volume_kvtxinmem.Config{Verbose: daemonFlags.InmemDBVerbose}
		confSet[id] = configset.NewControllerConfig(1, conf)
	}

	// Load defined badger databases
	for i, bdbi := range daemonFlags.BadgerDBs {
		id := "cli-badger-volume-" + strconv.Itoa(i)
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		conf := &volume_badger.Config{
			Dir: bdb,
		}
		confSet[id] = configset.NewControllerConfig(1, conf)
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
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
		go func() {
			le.Debugf("profiling listener running: %s", daemonFlags.ProfListen)
			err := http.ListenAndServe(daemonFlags.ProfListen, nil)
			le.WithError(err).Warn("profiling listener exited")
		}()
	}
	_ = d

	<-ctx.Done()
	return nil
}

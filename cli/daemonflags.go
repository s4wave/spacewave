package cli

import (
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	volume_badger "github.com/aperturerobotics/hydra/volume/badger"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	volume_redis "github.com/aperturerobotics/hydra/volume/redis"
	"github.com/urfave/cli"
)

// DaemonArgs contains common flags for hydra daemons.
type DaemonArgs struct {
	// BadgerDBs contains a list of badger db paths
	// use a YAML configuration file if you want to adjust options.
	BadgerDBs      cli.StringSlice
	InmemDB        bool
	InmemDBVerbose bool
	RedisURL       string
}

// BuildFlags attaches the flags to a flag set.
func (a *DaemonArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringSliceFlag{
			Name:   "badger-db",
			Usage:  "set a path to a badger db to load on startup",
			EnvVar: "HYDRA_BADGER_DB",
			Value:  &a.BadgerDBs,
		},
		cli.StringFlag{
			Name:        "redis-url",
			Usage:       "set a url to a redis instance to connect to on startup",
			EnvVar:      "HYDRA_REDIS_URL",
			Value:       a.RedisURL,
			Destination: &a.RedisURL,
		},
		cli.BoolFlag{
			Name:        "inmem-db",
			Usage:       "if set, start a in-memory volume on startup",
			EnvVar:      "HYDRA_INMEM_DB",
			Destination: &a.InmemDB,
		},
		cli.BoolFlag{
			Name:        "inmem-db-verbose",
			Usage:       "if set, mark inmem database as verbose. implies --inmem-db",
			EnvVar:      "HYDRA_INMEM_DB_VERBOSE",
			Destination: &a.InmemDBVerbose,
		},
	}
}

// ApplyToConfigSet applies the configured values to the configset.
func (a *DaemonArgs) ApplyToConfigSet(confSet configset.ConfigSet, overwrite bool) error {
	// Load defined inmem database
	if a.InmemDB || a.InmemDBVerbose {
		id := "cli-inmem-volume-0"
		conf := &volume_kvtxinmem.Config{Verbose: a.InmemDBVerbose}
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, conf)
		}
	}

	// Load defined badger databases
	for i, bdbi := range a.BadgerDBs {
		id := "cli-badger-volume-" + strconv.Itoa(i)
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_badger.Config{
				Dir: bdb,
			})
		}
	}

	if a.RedisURL != "" {
		id := "cli-redis-volume-0"
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_redis.Config{
				Url: a.RedisURL,
			})
		}
	}
	return nil
}

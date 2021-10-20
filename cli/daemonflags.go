package cli

import (
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	volume_badger "github.com/aperturerobotics/hydra/volume/badger"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
	volume_redis "github.com/aperturerobotics/hydra/volume/redis"
	"github.com/urfave/cli"
)

// CLIVolumeIDAlias is an alias applied to match the default CLI volume.
const CLIVolumeIDAlias = "hydra/volume/default"

// DaemonArgs contains common flags for hydra daemons.
type DaemonArgs struct {
	// BadgerDBs contains a list of badger db paths (directories)
	// use a YAML configuration file if you want to adjust options.
	BadgerDBs cli.StringSlice
	// BoltDBs contains a list of bolt db paths (files)
	// use a YAML configuration file if you want to adjust options.
	BoltDBs        cli.StringSlice
	BoltDBVerbose  bool
	InmemDB        bool
	InmemDBVerbose bool
	RedisURL       string
}

// BuildFlags attaches the flags to a flag set.
func (a *DaemonArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringSliceFlag{
			Name:   "badger-db",
			Usage:  "set a path to a badger db dir to load on startup",
			EnvVar: "HYDRA_BADGER_DB",
			Value:  &a.BadgerDBs,
		},
		cli.StringSliceFlag{
			Name:   "bolt-db",
			Usage:  "set a path to a bolt db file to load on startup",
			EnvVar: "HYDRA_BOLT_DB",
			Value:  &a.BoltDBs,
		},
		cli.BoolFlag{
			Name:        "bolt-db-verbose",
			Usage:       "if set, mark bolt database as verbose",
			EnvVar:      "HYDRA_BOLT_DB_VERBOSE",
			Destination: &a.BoltDBVerbose,
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
	// cliVolumeConfig is applied to all CLI volumes.
	cliVolumeConfig := &volume_controller.Config{
		VolumeIdAlias: []string{CLIVolumeIDAlias},
	}

	// Load defined inmem database
	if a.InmemDB || a.InmemDBVerbose {
		id := "cli-inmem-volume-0"
		conf := &volume_kvtxinmem.Config{
			Verbose:      a.InmemDBVerbose,
			VolumeConfig: cliVolumeConfig,
		}
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
				Dir:          bdb,
				VolumeConfig: cliVolumeConfig,
			})
		}
	}

	// Load defined bolt databases
	for i, bdbi := range a.BoltDBs {
		id := "cli-bolt-volume-" + strconv.Itoa(i)
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_bolt.Config{
				Path:         bdb,
				Verbose:      a.BoltDBVerbose,
				VolumeConfig: cliVolumeConfig,
			})
		}
	}

	if a.RedisURL != "" {
		id := "cli-redis-volume-0"
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_redis.Config{
				Url:          a.RedisURL,
				VolumeConfig: cliVolumeConfig,
			})
		}
	}
	return nil
}

// BuildSingleVolume builds a single volume from the given flags.
func (a *DaemonArgs) BuildSingleVolume() config.Config {
	cliVolumeConfig := &volume_controller.Config{
		VolumeIdAlias: []string{CLIVolumeIDAlias},
	}

	if a.RedisURL != "" {
		return &volume_redis.Config{
			Url:          a.RedisURL,
			VolumeConfig: cliVolumeConfig,
		}
	}

	// Load defined badger databases
	for _, bdbi := range a.BadgerDBs {
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		return &volume_badger.Config{
			Dir:          bdb,
			VolumeConfig: cliVolumeConfig,
		}
	}

	// Load defined bolt databases
	for _, bdbi := range a.BoltDBs {
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		return &volume_bolt.Config{
			Path:         bdb,
			Verbose:      a.BoltDBVerbose,
			VolumeConfig: cliVolumeConfig,
		}
	}

	if a.RedisURL != "" {
		return &volume_redis.Config{
			Url:          a.RedisURL,
			VolumeConfig: cliVolumeConfig,
		}
	}

	return &volume_kvtxinmem.Config{Verbose: a.InmemDBVerbose}
}

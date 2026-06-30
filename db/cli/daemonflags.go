//go:build !js && !wasip1

package cli

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	store_kvtx_redis "github.com/s4wave/spacewave/db/store/kvtx/redis"
	volume_badger "github.com/s4wave/spacewave/db/volume/badger"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_redis "github.com/s4wave/spacewave/db/volume/redis"
)

// CLIVolumeIDAlias is an alias applied to match the default CLI volume.
const CLIVolumeIDAlias = "default"

// DaemonArgs contains common flags for hydra daemons.
type DaemonArgs struct {
	// BadgerDBs contains a list of badger database directories.
	// Use a YAML configuration file to adjust options.
	BadgerDBs cli.StringSlice
	// BoltDBs contains a list of bolt database files.
	// Use a YAML configuration file to adjust options.
	BoltDBs cli.StringSlice
	// BoltDBVerbose marks bolt databases as verbose.
	BoltDBVerbose bool
	// InmemDB starts an in-memory volume.
	InmemDB bool
	// InmemDBVerbose marks the in-memory volume as verbose.
	InmemDBVerbose bool
	// RedisURL is the Redis instance URL to connect to on startup.
	RedisURL string
}

// BuildFlags attaches the flags to a flag set.
func (a *DaemonArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "badger-db",
			Usage:       "set a path to a badger db dir to load on startup",
			EnvVars:     []string{"HYDRA_BADGER_DB"},
			Value:       &a.BadgerDBs,
			Destination: &a.BadgerDBs,
		},
		&cli.StringSliceFlag{
			Name:        "bolt-db",
			Usage:       "set a path to a bolt db file to load on startup",
			EnvVars:     []string{"HYDRA_BOLT_DB"},
			Value:       &a.BoltDBs,
			Destination: &a.BoltDBs,
		},
		&cli.BoolFlag{
			Name:        "bolt-db-verbose",
			Usage:       "if set, mark bolt database as verbose",
			EnvVars:     []string{"HYDRA_BOLT_DB_VERBOSE"},
			Destination: &a.BoltDBVerbose,
		},
		&cli.StringFlag{
			Name:        "redis-url",
			Usage:       "set a url to a redis instance to connect to on startup",
			EnvVars:     []string{"HYDRA_REDIS_URL"},
			Value:       a.RedisURL,
			Destination: &a.RedisURL,
		},
		&cli.BoolFlag{
			Name:        "inmem-db",
			Usage:       "if set, start a in-memory volume on startup",
			EnvVars:     []string{"HYDRA_INMEM_DB"},
			Destination: &a.InmemDB,
		},
		&cli.BoolFlag{
			Name:        "inmem-db-verbose",
			Usage:       "if set, mark inmem database as verbose. implies --inmem-db",
			EnvVars:     []string{"HYDRA_INMEM_DB_VERBOSE"},
			Destination: &a.InmemDBVerbose,
		},
	}
}

// ApplyToConfigSet applies the configured values to the configset.
//
// If baseVolCtrlConf is nil, a default volume controller config is used.
func (a *DaemonArgs) ApplyToConfigSet(confSet configset.ConfigSet, overwrite bool, baseVolCtrlConf *volume_controller.Config) error {
	if baseVolCtrlConf == nil {
		baseVolCtrlConf = &volume_controller.Config{}
	}
	baseVolCtrlConf.VolumeIdAlias = append(baseVolCtrlConf.VolumeIdAlias, CLIVolumeIDAlias)

	if a.InmemDB || a.InmemDBVerbose {
		id := "cli-inmem-volume-0"
		conf := &volume_kvtxinmem.Config{
			Verbose:      a.InmemDBVerbose,
			VolumeConfig: baseVolCtrlConf,
		}
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, conf)
		}
	}

	for i, bdbi := range a.BadgerDBs.Value() {
		id := "cli-badger-volume-" + strconv.Itoa(i)
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_badger.Config{
				Dir:          bdb,
				VolumeConfig: baseVolCtrlConf,
			})
		}
	}

	for i, bdbi := range a.BoltDBs.Value() {
		id := "cli-bolt-volume-" + strconv.Itoa(i)
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_bolt.Config{
				Path:         bdb,
				Verbose:      a.BoltDBVerbose,
				VolumeConfig: baseVolCtrlConf,
			})
		}
	}

	if a.RedisURL != "" {
		id := "cli-redis-volume-0"
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_redis.Config{
				Client: &store_kvtx_redis.ClientConfig{
					Url: a.RedisURL,
				},
				VolumeConfig: baseVolCtrlConf,
			})
		}
	}
	return nil
}

// BuildSingleVolume builds a single volume from the given flags.
//
// id is optional and specifies a prefix to use for the volume. If
// baseVolCtrlConf is nil, a default volume controller config is used.
func (a *DaemonArgs) BuildSingleVolume(id string, baseVolCtrlConf *volume_controller.Config) config.Config {
	if baseVolCtrlConf == nil {
		baseVolCtrlConf = &volume_controller.Config{}
	}
	baseVolCtrlConf.VolumeIdAlias = append(baseVolCtrlConf.VolumeIdAlias, CLIVolumeIDAlias)

	id = strings.TrimSpace(id)

	for _, bdbi := range a.BadgerDBs.Value() {
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		dir := bdb
		if id != "" {
			dir = filepath.Join(dir, id)
		}

		return &volume_badger.Config{
			Dir:          dir,
			VolumeConfig: baseVolCtrlConf,
		}
	}

	for _, bdbi := range a.BoltDBs.Value() {
		bdb := strings.TrimSpace(bdbi)
		if bdb == "" {
			continue
		}

		if id != "" {
			dir := filepath.Dir(bdb)
			fileName := filepath.Base(bdb)
			ext := filepath.Ext(fileName)
			nameWithoutExt := strings.TrimSuffix(fileName, ext)
			bdb = filepath.Join(dir, nameWithoutExt+"-"+id+ext)
		}

		return &volume_bolt.Config{
			Path:         bdb,
			Verbose:      a.BoltDBVerbose,
			VolumeConfig: baseVolCtrlConf,
		}
	}

	if a.RedisURL != "" {
		// Redis volume construction has no id-scoped path component.
		return &volume_redis.Config{
			Client: &store_kvtx_redis.ClientConfig{
				Url: a.RedisURL,
			},
			VolumeConfig: baseVolCtrlConf,
		}
	}

	return &volume_kvtxinmem.Config{Verbose: a.InmemDBVerbose, VolumeConfig: baseVolCtrlConf}
}

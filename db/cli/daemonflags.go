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
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_redis "github.com/s4wave/spacewave/db/volume/redis"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// CLIVolumeIDAlias is an alias applied to match the default CLI volume.
const CLIVolumeIDAlias = "default"

// DaemonArgs contains common flags for hydra daemons.
type DaemonArgs struct {
	// BadgerDBs contains a list of badger database directories.
	// Use a YAML configuration file to adjust options.
	BadgerDBs cli.StringSlice
	// S4dbs contains a list of s4db database files.
	// Use a YAML configuration file to adjust options.
	S4dbs cli.StringSlice
	// S4dbVerbose marks s4db databases as verbose.
	S4dbVerbose bool
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
			Name:        "s4db",
			Usage:       "set a path to an s4db file to load on startup",
			EnvVars:     []string{"HYDRA_S4DB"},
			Value:       &a.S4dbs,
			Destination: &a.S4dbs,
		},
		&cli.BoolFlag{
			Name:        "s4db-verbose",
			Usage:       "if set, mark s4db databases as verbose",
			EnvVars:     []string{"HYDRA_S4DB_VERBOSE"},
			Destination: &a.S4dbVerbose,
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
	// Prepare the shared volume configuration and CLI alias.
	if baseVolCtrlConf == nil {
		baseVolCtrlConf = &volume_controller.Config{}
	}
	baseVolCtrlConf.VolumeIdAlias = append(baseVolCtrlConf.VolumeIdAlias, CLIVolumeIDAlias)

	// Register the optional in-memory volume.
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

	// Register each configured Badger volume.
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

	// Register each configured s4db volume.
	for i, pathi := range a.S4dbs.Value() {
		id := "cli-s4db-volume-" + strconv.Itoa(i)
		path := strings.TrimSpace(pathi)
		if path == "" {
			continue
		}
		if _, ok := confSet[id]; !ok || overwrite {
			confSet[id] = configset.NewControllerConfig(1, &volume_s4db.Config{
				Path:         path,
				Verbose:      a.S4dbVerbose,
				VolumeConfig: baseVolCtrlConf,
			})
		}
	}

	// Register the optional Redis volume.
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
	// Prepare the shared volume configuration and normalize the volume ID.
	if baseVolCtrlConf == nil {
		baseVolCtrlConf = &volume_controller.Config{}
	}
	baseVolCtrlConf.VolumeIdAlias = append(baseVolCtrlConf.VolumeIdAlias, CLIVolumeIDAlias)
	id = strings.TrimSpace(id)

	// Build a Badger volume when a database directory is configured.
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

	// Build an s4db volume when a database file is configured.
	for _, pathi := range a.S4dbs.Value() {
		path := strings.TrimSpace(pathi)
		if path == "" {
			continue
		}
		if id != "" {
			ext := filepath.Ext(path)
			path = strings.TrimSuffix(path, ext) + "-" + id + ext
		}
		return &volume_s4db.Config{
			Path:         path,
			Verbose:      a.S4dbVerbose,
			VolumeConfig: baseVolCtrlConf,
		}
	}

	// Build a Redis volume when a Redis URL is configured.
	if a.RedisURL != "" {
		// Redis volume construction has no id-scoped path component.
		return &volume_redis.Config{
			Client: &store_kvtx_redis.ClientConfig{
				Url: a.RedisURL,
			},
			VolumeConfig: baseVolCtrlConf,
		}
	}

	// Fall back to the in-memory volume configuration.
	return &volume_kvtxinmem.Config{Verbose: a.InmemDBVerbose, VolumeConfig: baseVolCtrlConf}
}

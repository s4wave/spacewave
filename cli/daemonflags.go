package cli

import (
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/hydra/volume/badger"
	"github.com/aperturerobotics/hydra/volume/kvtxinmem"
	"github.com/urfave/cli"
)

// DaemonArgs contains common flags for hydra daemons.
type DaemonArgs struct {
	// BadgerDBs contains a list of badger db paths
	// use a YAML configuration file if you want to adjust options.
	BadgerDBs      cli.StringSlice
	InmemDB        bool
	InmemDBVerbose bool
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
func (a *DaemonArgs) ApplyToConfigSet(confSet configset.ConfigSet, overwrite bool) {
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
}

//go:build !js

package spacewave_cli

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// CollectorFrees names the Volumes whose collector frees unreachable blocks,
// as serve's --collector-frees flag sets them. Serve parses its flags before
// it builds the bus, and ConfigureVolume reads the names as each Volume
// opens. Every other Volume, and every Volume a process other than serve
// opens, keeps frees off. The Volume records the switch: an open that turns
// frees on returns it to unadopted, so it counts every holder again before
// its first free, and an open with frees off turns them off.
type CollectorFrees struct {
	// volumes are the named Volumes, each as its file name without the
	// extension.
	volumes cli.StringSlice
}

// NewCollectorFrees constructs a CollectorFrees naming no Volume.
func NewCollectorFrees() *CollectorFrees {
	return &CollectorFrees{}
}

// BuildFlags returns the flag naming the Volumes that free.
func (f *CollectorFrees) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "collector-frees",
			Usage:       "free unreachable blocks on the named volume (its file name without .s4wave); restart without it to turn frees off",
			Destination: &f.volumes,
		},
	}
}

// ConfigureVolume turns frees on, under the hold-until-free rule they run
// under, for a named s4db Volume.
func (f *CollectorFrees) ConfigureVolume(id string, conf config.Config) {
	// Find a named s4db Volume by its file name.
	volumeConf, ok := conf.(*volume_s4db.Config)
	if !ok {
		return
	}
	name := strings.TrimSuffix(filepath.Base(volumeConf.GetPath()), storage_native.VolumeExt)
	if !slices.Contains(f.volumes.Value(), name) {
		return
	}

	// Turn frees on in its store config, keeping the rest.
	storeConf := volumeConf.GetStoreConfig().CloneVT()
	if storeConf == nil {
		storeConf = &store_kvtx.Config{}
	}
	storeConf.CollectorFrees = true
	storeConf.CollectorHoldUntilFree = true
	volumeConf.StoreConfig = storeConf
}

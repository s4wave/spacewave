//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
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
	// trustListed takes a source's recorded refs as read from content, as
	// block.SetTrustListedRefs describes.
	trustListed bool
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
		&cli.BoolFlag{
			Name: "trust-listed-refs",
			Usage: "copy blocks with the child lists their source recorded as if read from content, the old behavior; " +
				"refused with --collector-frees, since such a list may lack a child a free would then take. " +
				"Verify the copied lists with debug ref-repair before turning frees on",
			Destination: &f.trustListed,
		},
	}
}

// Check logs the named Volumes and refuses a name with no Volume file under
// root, the state root the bus opens its Volumes in, empty when the bus has
// none: a mistyped name would otherwise leave every Volume's frees off while
// the operator believes collection runs. It refuses --trust-listed-refs with
// any named Volume, and otherwise sets it for the process.
func (f *CollectorFrees) Check(le *logrus.Entry, root string) error {
	// Refuse trusting listed refs while any Volume frees, then set it.
	names := f.volumes.Value()
	if f.trustListed {
		if len(names) != 0 {
			return errors.New("trust-listed-refs: refused with collector-frees, since a copied list may lack a child a free would take")
		}
		block.SetTrustListedRefs(true)
		le.Warn("trusting listed refs as read from content; verify copied lists with debug ref-repair before turning frees on")
	}

	// Name no Volume, and check nothing, without the flag.
	if len(names) == 0 {
		return nil
	}
	le.WithField("volumes", names).Info("collector frees named")

	// Refuse a name with no file, as work on the wrong target.
	if root == "" {
		return errors.New("collector-frees: the bus has no state root to find the volumes in")
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name+storage_native.VolumeExt)); err != nil {
			return errors.Wrapf(err, "collector-frees %q", name)
		}
	}
	return nil
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
	volumeConf.StoreConfig = storeConf
}

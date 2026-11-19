//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/config"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// upgradeFormatEnv opts in to format upgrades, as --upgrade-format does.
const upgradeFormatEnv = "SPACEWAVE_UPGRADE_FORMAT"

// FormatUpgrade decides whether this process may upgrade the stored format of
// a Volume that holds data. An upgrade is one way, and a development CLI may
// write a format that no release reads, so the development CLI refuses such a
// Volume with volume.ErrFormatUpgradeRefused unless the operator passes
// --upgrade-format or sets SPACEWAVE_UPGRADE_FORMAT=1. The app, a
// distribution entrypoint in dev and release builds, upgrades on its own,
// since a person cannot pass it a flag. Serve applies it to every Volume its
// bus opens, and the debug commands that write a stopped Volume to that
// Volume. Nothing else sets the store's refusal, so every other opener
// upgrades as before.
type FormatUpgrade struct {
	// distribution reports whether a distribution entrypoint runs the
	// process, nil for never. The entrypoint sets it after the commands are
	// built, so it is read when a Volume opens.
	distribution func() bool
	// allow is the operator's opt-in.
	allow bool
}

// NewFormatUpgrade constructs a FormatUpgrade. distribution reports whether a
// distribution entrypoint runs the process; nil treats it as the development
// CLI.
func NewFormatUpgrade(distribution func() bool) *FormatUpgrade {
	return &FormatUpgrade{distribution: distribution}
}

// BuildFlags returns the opt-in flag.
func (f *FormatUpgrade) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "upgrade-format",
			Usage:       "upgrade a volume stored at an older format, which the development CLI otherwise refuses; the upgrade is one way",
			EnvVars:     []string{upgradeFormatEnv},
			Destination: &f.allow,
		},
	}
}

// Refuses reports whether a Volume with data at an older format stays as it
// was instead of upgrading.
func (f *FormatUpgrade) Refuses() bool {
	return !f.allow && (f.distribution == nil || !f.distribution())
}

// StoreConfig returns conf, nil for the default, with the refusal set.
func (f *FormatUpgrade) StoreConfig(conf *store_kvtx.Config) *store_kvtx.Config {
	conf = conf.CloneVT()
	if conf == nil {
		conf = &store_kvtx.Config{}
	}
	conf.RefuseFormatUpgrade = f.Refuses()
	return conf
}

// ConfigureVolume sets the refusal on an s4db Volume's store config.
func (f *FormatUpgrade) ConfigureVolume(_ string, conf config.Config) {
	volumeConf, ok := conf.(*volume_s4db.Config)
	if !ok {
		return
	}
	volumeConf.StoreConfig = f.StoreConfig(volumeConf.GetStoreConfig())
}

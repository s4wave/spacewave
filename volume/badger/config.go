package volume_badger

import (
	"github.com/aperturerobotics/controllerbus/config"
	bdb "github.com/dgraph-io/badger"
	bdbopts "github.com/dgraph-io/badger/options"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

var opts bdb.Options

// BuildBadgerOptions builds badger options from the config.
func (c *Config) BuildBadgerOptions() (*bdb.Options, error) {
	o := bdb.DefaultOptions
	if c.GetDir() == "" {
		return nil, errors.New("db dir cannot be empty")
	}
	o.Dir = c.GetDir()
	if vd := c.GetValueDir(); vd != "" {
		o.ValueDir = vd
	} else {
		o.ValueDir = o.Dir
	}

	if c.GetNoSyncWrites() {
		o.SyncWrites = false
	}

	var err error
	o.TableLoadingMode, err = c.GetTableLoadingMode().ToLoadingMode(o.TableLoadingMode)
	if err != nil {
		return nil, err
	}

	o.ValueLogLoadingMode, err = c.GetValueLogLoadingMode().ToLoadingMode(o.ValueLogLoadingMode)
	if err != nil {
		return nil, err
	}

	if nvc := c.GetNumVersionsToKeep(); nvc != 0 {
		o.NumVersionsToKeep = int(nvc)
	}
	if mts := c.GetMaxTableSize(); mts != 0 {
		o.MaxTableSize = int64(mts)
	}
	if lsm := c.GetLevelSizeMultiplier(); lsm != 0 {
		o.LevelSizeMultiplier = int(lsm)
	}
	if ml := c.GetMaxLevels(); ml != 0 {
		o.MaxLevels = int(ml)
	}
	if vt := c.GetValueThreshold(); vt != 0 {
		o.ValueThreshold = int(vt)
	}
	if nmt := c.GetNumMemtables(); nmt != 0 {
		o.NumMemtables = int(nmt)
	}
	if nlzt := c.GetNumLevelZeroTables(); nlzt != 0 {
		o.NumLevelZeroTables = int(nlzt)
	}
	if nlzts := c.GetNumLevelZeroTablesStall(); nlzts != 0 {
		o.NumLevelZeroTablesStall = int(nlzts)
	}
	if los := c.GetLevelOneSize(); los != 0 {
		o.LevelOneSize = int64(los)
	}
	if vlfs := c.GetValueLogFileSize(); vlfs != 0 {
		o.ValueLogFileSize = int64(vlfs)
	}
	if vlme := c.GetValueLogMaxEntries(); vlme != 0 {
		o.ValueLogMaxEntries = vlme
	}
	if nc := c.GetNumCompactors(); nc != 0 {
		o.NumCompactors = int(nc)
	}
	if t := c.GetTruncate(); t {
		o.Truncate = c.GetTruncate()
	}
	return &o, nil
}

// ToLoadingMode interpets the option to a badger file loading mode.
func (f FileLoadingMode) ToLoadingMode(
	defMode bdbopts.FileLoadingMode,
) (bdbopts.FileLoadingMode, error) {
	switch f {
	case FileLoadingMode_FileLoadingMode_DEFAULT:
		return defMode, nil
	case FileLoadingMode_FileLoadingMode_FileIO:
		return bdbopts.FileIO, nil
	case FileLoadingMode_FileLoadingMode_LoadToRAM:
		return bdbopts.LoadToRAM, nil
	case FileLoadingMode_FileLoadingMode_MemoryMap:
		return bdbopts.MemoryMap, nil
	default:
	}

	return bdbopts.FileLoadingMode(0),
		errors.Errorf("unrecognized file loading mode: %s", f.String())
}

// Validate validates the configuration.
// This is a cursory validation to see if the values "look correct."
func (c *Config) Validate() error {
	if _, err := c.BuildBadgerOptions(); err != nil {
		return err
	}

	return nil
}

// GetConfigID returns the unique string for this configuration type.
// This string is stored with the encoded config.
func (c *Config) GetConfigID() string {
	return ControllerID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	ot, ok := other.(*Config)
	if !ok {
		return false
	}

	return proto.Equal(c, ot)
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))

package volume_badger

import (
	"github.com/aperturerobotics/controllerbus/config"
	bdb "github.com/dgraph-io/badger/v4"
	"github.com/pkg/errors"
)

// ConfigID is the id attached to the config objects.
var ConfigID = ControllerID

// BuildBadgerOptions builds badger options from the config.
func (c *Config) BuildBadgerOptions() (*bdb.Options, error) {
	if c.GetDir() == "" {
		return nil, errors.New("db dir cannot be empty")
	}
	o := bdb.DefaultOptions(c.GetDir())
	if vd := c.GetValueDir(); vd != "" {
		o.ValueDir = vd
	} else {
		o.ValueDir = o.Dir
	}

	// We use a write mutex, so conflict checking is unnecessary.
	o.DetectConflicts = false

	if c.GetNoSyncWrites() {
		o.SyncWrites = false
	}

	if nvc := c.GetNumVersionsToKeep(); nvc != 0 {
		o.NumVersionsToKeep = int(nvc)
	}
	if bts := c.GetBaseTableSize(); bts != 0 {
		o.BaseTableSize = int64(bts)
	}
	if lsm := c.GetLevelSizeMultiplier(); lsm != 0 {
		o.LevelSizeMultiplier = int(lsm)
	}
	if ml := c.GetMaxLevels(); ml != 0 {
		o.MaxLevels = int(ml)
	}
	if vt := c.GetValueThreshold(); vt != 0 {
		o.ValueThreshold = int64(vt)
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
	if los := c.GetBaseLevelSize(); los != 0 {
		o.BaseLevelSize = int64(los)
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

	return &o, nil
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
	return config.EqualsConfig[*Config](c, other)
}

// GetDebugVals returns the directive arguments as key/value pairs.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (c *Config) GetDebugVals() config.DebugValues {
	vals := make(config.DebugValues)
	if dir := c.GetDir(); dir != "" {
		vals["dir"] = []string{dir}
	}
	if valueDir := c.GetValueDir(); valueDir != "" {
		vals["value-dir"] = []string{valueDir}
	}
	return vals
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))

// _ is a type assertion
var _ config.Debuggable = ((*Config)(nil))

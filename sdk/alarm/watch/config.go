package alarm_watch

import (
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ConfigID is the string used to identify this config object.
const ConfigID = "alarm/watch"

// Validate requires the World engine, the Alarm, the function and a hold
// time that is not negative, and a source for any queries.
func (c *Config) Validate() error {
	// Require the objects the watch reads and writes.
	if c.GetEngineId() == "" {
		return world.ErrEmptyEngineID
	}
	if c.GetAlarmKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "alarm")
	}
	if c.GetFunctionKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "function")
	}

	// Require a hold time that is not negative.
	if c.GetHoldMs() < 0 {
		return errors.New("the hold time is negative")
	}

	// Require a source for the queries, each with a unique name.
	if len(c.GetQueries()) != 0 && c.GetSourceKey() == "" {
		return errors.New("queries need a source object")
	}
	names := make(map[string]struct{}, len(c.GetQueries()))
	for _, q := range c.GetQueries() {
		if q.GetName() == "" || q.GetExpr() == "" {
			return errors.New("each query needs a name and an expression")
		}
		if _, dup := names[q.GetName()]; dup {
			return errors.Errorf("query %q is named twice", q.GetName())
		}
		names[q.GetName()] = struct{}{}
	}
	return nil
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the other config is equal.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig(c, other)
}

// _ is a type assertion
var _ config.Config = (*Config)(nil)

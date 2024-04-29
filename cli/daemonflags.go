//go:build !js && !wasip1

package cli

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/urfave/cli/v2"
)

// DaemonArgs contains common flags for forge daemons.
type DaemonArgs struct{}

// BuildFlags attaches the flags to a flag set.
func (a *DaemonArgs) BuildFlags() []cli.Flag {
	return nil
}

// ApplyToConfigSet applies the configured values to the configset.
func (a *DaemonArgs) ApplyToConfigSet(confSet configset.ConfigSet, overwrite bool) error {
	return nil
}

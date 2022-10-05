package plugin_fetch

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// Config is a configuration for a PluginFetch Controller.
type Config interface {
	// Config is the base config interface.
	config.Config

	// SetFetchPluginIdRegex sets the regex of plugin IDs to fetch with this controller.
	SetFetchPluginIdRegex(re string)
}

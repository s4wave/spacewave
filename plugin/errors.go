package plugin

import "errors"

var (
	// ErrPluginIdEmpty is returned if the plugin id was empty.
	ErrPluginIdEmpty = errors.New("plugin id cannot be empty")
)

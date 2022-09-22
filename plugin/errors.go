package plugin

import "errors"

var (
	// ErrEmptyPluginID is returned if the plugin ID was empty.
	ErrEmptyPluginID = errors.New("plugin id cannot be empty")
)

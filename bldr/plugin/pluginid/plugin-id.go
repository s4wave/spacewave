package pluginid

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/labels"
)

// ErrEmptyPluginID is returned if the plugin ID was empty.
var ErrEmptyPluginID = errors.New("plugin id cannot be empty")

// Validate validates a Bldr plugin ID.
func Validate(id string, allowEmpty bool) error {
	if id == "" {
		if allowEmpty {
			return nil
		}
		return ErrEmptyPluginID
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "invalid plugin id")
	}
	return nil
}

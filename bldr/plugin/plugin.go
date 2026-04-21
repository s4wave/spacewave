package bldr_plugin

import (
	"github.com/s4wave/spacewave/net/util/labels"
	"github.com/pkg/errors"
)

// ValidatePluginID validates a plugin ID.
func ValidatePluginID(id string, allowEmpty bool) error {
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

// Validate validates the PluginStatus object.
func (s *PluginStatus) Validate() error {
	if err := ValidatePluginID(s.GetPluginId(), false); err != nil {
		return err
	}
	return nil
}

// Validate validates the LoadPlugin request.
func (r *LoadPluginRequest) Validate() error {
	if err := ValidatePluginID(r.GetPluginId(), false); err != nil {
		return err
	}
	return nil
}

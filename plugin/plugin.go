package plugin

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/pkg/errors"
)

// ValidatePluginID validates a plugin ID.
func ValidatePluginID(id string) error {
	if id == "" {
		return ErrPluginIdEmpty
	}
	if err := labels.ValidateDNSLabel(id); err != nil {
		return errors.Wrap(err, "plugin id")
	}
	return nil
}

// Validate validates the PluginStatus object.
func (s *PluginStatus) Validate() error {
	if err := ValidatePluginID(s.GetPluginId()); err != nil {
		return err
	}
	return nil
}

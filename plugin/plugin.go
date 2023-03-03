package bldr_plugin

import (
	"github.com/aperturerobotics/bifrost/util/labels"
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
		return errors.Wrap(err, "plugin id")
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

// Validate validates the FetchPlugin request.
func (r *FetchPluginRequest) Validate() error {
	if err := ValidatePluginID(r.GetPluginId(), false); err != nil {
		return err
	}
	return nil
}

// Validate validates the FetchPlugin response.
func (r *FetchPluginResponse) Validate() error {
	if r.GetPluginManifest().GetEmpty() {
		return errors.New("plugin manifest: cannot be empty")
	}
	if err := r.GetPluginManifest().Validate(); err != nil {
		return errors.Wrap(err, "plugin_manifest")
	}
	return nil
}

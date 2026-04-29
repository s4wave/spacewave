package bldr_plugin

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/labels"
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

// BuildPluginRpcComponentID builds the PluginRpc component id.
func BuildPluginRpcComponentID(pluginID, instanceKey string) string {
	if instanceKey == "" {
		return pluginID
	}
	return pluginID + "/" + instanceKey
}

// ParsePluginRpcComponentID parses the PluginRpc component id.
func ParsePluginRpcComponentID(componentID string) (string, string) {
	pluginID, instanceKey, _ := strings.Cut(componentID, "/")
	return pluginID, instanceKey
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

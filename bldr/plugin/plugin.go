package bldr_plugin

import (
	"net/url"
	"strings"

	"github.com/s4wave/spacewave/bldr/plugin/pluginid"
)

// ValidatePluginID validates a plugin ID.
func ValidatePluginID(id string, allowEmpty bool) error {
	return pluginid.Validate(id, allowEmpty)
}

// BuildPluginRpcComponentID addresses a plugin binding and optional exact executable.
func BuildPluginRpcComponentID(pluginID, instanceKey, manifestRoot string) string {
	componentID := pluginID
	if instanceKey != "" {
		componentID += "/" + url.PathEscape(instanceKey)
	}
	if manifestRoot != "" {
		componentID += "?manifest=" + url.QueryEscape(manifestRoot)
	}
	return componentID
}

// ParsePluginRpcComponentID keeps executable identity separate from instance routing.
func ParsePluginRpcComponentID(componentID string) (pluginID, instanceKey, manifestRoot string, err error) {
	path, query, _ := strings.Cut(componentID, "?")
	pluginID, instanceKey, _ = strings.Cut(path, "/")
	instanceKey, err = url.PathUnescape(instanceKey)
	if err != nil {
		return
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return
	}
	manifestRoot = values.Get("manifest")
	return
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

package bldr_plugin

import (
	"encoding/base64"

	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/pkg/errors"
)

// NewPluginStartInfo constructs a new PluginStartInfo.
func NewPluginStartInfo(instanceID, pluginID, instanceKey string) *PluginStartInfo {
	return &PluginStartInfo{
		InstanceId:  instanceID,
		PluginId:    pluginID,
		InstanceKey: instanceKey,
	}
}

// UnmarshalPluginStartInfoJsonBase64 unmarshals a base64-encoded JSON string into a PluginStartInfo.
func UnmarshalPluginStartInfoJsonBase64(data string) (*PluginStartInfo, error) {
	jdat, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, errors.Wrap(err, "decode base64")
	}

	startInfo := &PluginStartInfo{}
	if err := startInfo.UnmarshalJSON(jdat); err != nil {
		return nil, errors.Wrap(err, "unmarshal json")
	}

	return startInfo, nil
}

// Validate checks the plugin meta.
func (m *PluginStartInfo) Validate() error {
	if err := labels.ValidateDNSLabel(m.GetInstanceId()); err != nil {
		return errors.Wrap(err, "instance_id")
	}
	if err := ValidatePluginID(m.GetPluginId(), true); err != nil {
		return errors.Wrap(err, "plugin_id")
	}
	return nil
}

// MarshalJsonBase64 marshals the start info to base64-encoded json.
func (m *PluginStartInfo) MarshalJsonBase64() (string, error) {
	jdat, err := m.MarshalJSON()
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(jdat), nil
}

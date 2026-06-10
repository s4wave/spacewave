package bldr_plugin

import (
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/util/labels"
)

// NewPluginMeta constructs a new PluginMeta.
func NewPluginMeta(projectID, pluginID, platformID, buildType string) *PluginMeta {
	return &PluginMeta{
		ProjectId:  projectID,
		PluginId:   pluginID,
		PlatformId: platformID,
		BuildType:  buildType,
	}
}

// UnmarshalPluginMetaB58 unmarshals a b58 meta.
// Note: we compress with gzip compression.
func UnmarshalPluginMetaB58(str string) (*PluginMeta, error) {
	m := &PluginMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate checks the plugin meta.
func (m *PluginMeta) Validate() error {
	if err := labels.ValidateDNSLabel(m.GetProjectId()); err != nil {
		return errors.Wrap(err, "project_id")
	}
	if err := labels.ValidateDNSLabel(m.GetPluginId()); err != nil {
		return errors.Wrap(err, "plugin_id")
	}
	return nil
}

// MarshalB58 marshals the conf to a b58 string.
// note: plugin metadata is base58 encoded protobuf data.
func (m *PluginMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

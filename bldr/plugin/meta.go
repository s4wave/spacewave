package bldr_plugin

import (
	b58 "github.com/mr-tron/base58/base58"
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
// The metadata uses gzip compression.
func UnmarshalPluginMetaB58(str string) (*PluginMeta, error) {
	// Decode the base58 payload containing the plugin metadata.
	m := &PluginMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}

	// Decode the plugin metadata from its protobuf payload.
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// MarshalB58 marshals the conf to a b58 string.
// note: plugin metadata is base58 encoded protobuf data.
func (m *PluginMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

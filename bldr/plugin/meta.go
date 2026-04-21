package bldr_plugin

import (
	"github.com/s4wave/spacewave/net/util/labels"
	"github.com/klauspost/compress/s2"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
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
// Note: we compress with s2 compression.
func UnmarshalPluginMetaB58(str string) (*PluginMeta, error) {
	m := &PluginMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	data, err = s2.Decode(nil, data)
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
// note: we compress with s2 compression & encrypt with a psk.
func (m *PluginMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	dat = s2.EncodeBest(nil, dat)
	return b58.Encode(dat)
}

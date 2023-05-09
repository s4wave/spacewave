package bldr_plugin

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/klauspost/compress/s2"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
)

// NewPluginStartInfo constructs a new PluginStartInfo.
func NewPluginStartInfo(instanceID string) *PluginStartInfo {
	return &PluginStartInfo{
		InstanceId: instanceID,
	}
}

// UnmarshalPluginStartInfoB58 unmarshals a b58 meta.
// Note: we compress with s2 compression.
func UnmarshalPluginStartInfoB58(str string) (*PluginStartInfo, error) {
	m := &PluginStartInfo{}
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
func (m *PluginStartInfo) Validate() error {
	if err := labels.ValidateDNSLabel(m.GetInstanceId()); err != nil {
		return errors.Wrap(err, "instance_id")
	}
	return nil
}

// MarshalB58 marshals the conf to a b58 string.
// note: we compress with s2 compression & encrypt with a psk.
func (m *PluginStartInfo) MarshalB58() string {
	dat, _ := m.MarshalVT()
	dat = s2.EncodeBest(nil, dat)
	return b58.Encode(dat)
}

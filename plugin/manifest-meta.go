package bldr_plugin

import (
	b58 "github.com/mr-tron/base58/base58"
	"github.com/sirupsen/logrus"
)

// NewPluginManifestMeta constructs a new PluginManifestMeta.
func NewPluginManifestMeta(pluginID string, buildType BuildType, pluginPlatformID string, pluginRev uint64) *PluginManifestMeta {
	return &PluginManifestMeta{
		PluginId:         pluginID,
		BuildType:        string(buildType),
		PluginPlatformId: pluginPlatformID,
		Rev:              pluginRev,
	}
}

// UnmarshalPluginManifestMetaB58 unmarshals a b58 plugin manifest meta.
func UnmarshalPluginManifestMetaB58(str string) (*PluginManifestMeta, error) {
	m := &PluginManifestMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// MustUnmarshalPluginManifestB58 unmarshals ignoring any error.
func MustUnmarshalPluginManifestB58(str string) *PluginManifestMeta {
	v, _ := UnmarshalPluginManifestMetaB58(str)
	return v
}

// Validate validates the PluginManifestMeta.
func (m *PluginManifestMeta) Validate(allowEmpty bool) error {
	if err := ValidatePluginID(m.GetPluginId(), allowEmpty); err != nil {
		return ErrEmptyPluginID
	}
	// ignore unknown build types & plugin platform ids
	/*
		if err := ToBuildType(m.GetBuildType()).Validate(false); err != nil {
			return err
		}
	*/
	return nil
}

// MarshalB58 marshals the meta to a b58 string.
func (m *PluginManifestMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

// MarshalBlock marshals the block to binary.
func (m *PluginManifestMeta) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *PluginManifestMeta) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// Logger adds logging fields for the meta.
func (m *PluginManifestMeta) Logger(le *logrus.Entry) *logrus.Entry {
	fields := logrus.Fields{}
	if pluginID := m.GetPluginId(); pluginID != "" {
		fields["plugin-id"] = pluginID
	}
	if buildType := m.GetBuildType(); buildType != "" {
		fields["build-type"] = buildType
	}
	if pluginPlatformID := m.GetPluginPlatformId(); pluginPlatformID != "" {
		fields["plugin-platform"] = pluginPlatformID
	}
	if rev := m.GetRev(); rev != 0 {
		fields["plugin-rev"] = rev
	}
	if len(fields) == 0 {
		return le
	}
	return le.WithFields(fields)
}

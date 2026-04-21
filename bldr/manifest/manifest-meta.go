package bldr_manifest

import (
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/sirupsen/logrus"
)

// NewManifestMeta constructs a new ManifestMeta.
func NewManifestMeta(manifestID string, buildType BuildType, platformID string, rev uint64) *ManifestMeta {
	return &ManifestMeta{
		ManifestId: manifestID,
		BuildType:  string(buildType),
		PlatformId: platformID,
		Rev:        rev,
	}
}

// UnmarshalManifestMetaB58 unmarshals a b58 manifest meta.
func UnmarshalManifestMetaB58(str string) (*ManifestMeta, error) {
	m := &ManifestMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// MustUnmarshalManifestB58 unmarshals ignoring any error.
func MustUnmarshalManifestB58(str string) *ManifestMeta {
	v, _ := UnmarshalManifestMetaB58(str)
	return v
}

// Resolve resolves the platform ID to a fully qualified ID.
//
// returns a copy of the meta object w/ the platform id set.
func (m *ManifestMeta) Resolve() (*ManifestMeta, bldr_platform.Platform, error) {
	meta := m.CloneVT()

	// parse platform id
	buildPlatform, err := bldr_platform.ParsePlatform(meta.GetPlatformId())
	if err != nil {
		return nil, nil, err
	}
	meta.PlatformId = buildPlatform.GetPlatformID()
	return meta, buildPlatform, nil
}

// Validate validates the ManifestMeta.
func (m *ManifestMeta) Validate(allowEmpty bool) error {
	if err := ValidateManifestID(m.GetManifestId(), allowEmpty); err != nil {
		return err
	}
	return nil
}

// MarshalB58 marshals the meta to a b58 string.
func (m *ManifestMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

// MarshalBlock marshals the block to binary.
func (m *ManifestMeta) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (m *ManifestMeta) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

// Logger adds logging fields for the meta.
func (m *ManifestMeta) Logger(le *logrus.Entry) *logrus.Entry {
	fields := logrus.Fields{}
	if manifestID := m.GetManifestId(); manifestID != "" {
		fields["manifest-id"] = manifestID
	}
	if buildType := m.GetBuildType(); buildType != "" {
		fields["build-type"] = buildType
	}
	if platformID := m.GetPlatformId(); platformID != "" {
		fields["platform-id"] = platformID
	}
	if rev := m.GetRev(); rev != 0 {
		fields["manifest-rev"] = rev
	}
	if len(fields) == 0 {
		return le
	}
	return le.WithFields(fields)
}
